package perscom_events

import (
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

/*
**temporaryPassRequestEventListener**
Handles the "Temporary Pass" button click.
Sends an ephemeral embed to the user with a description and a "Submit" button.

**temporaryPassRequestSubmitEventListener**
Handles the "Submit" button in the ephemeral message.
Creates a pending request and posts it to the approval channel with Approve/Deny buttons.
Notifies the user that their request was submitted via ephemeral embed.

**tprApprovalListener**
Handles Approve/Deny button clicks by admins.
Approve:
Moves the request from pending to the approved list in the forum post.
Updates the forum post message with each new approval.
DMs the user that their request was approved.
Deletes the approval message from the channel.
Deny:
Opens a modal for the admin to enter a denial reason.

**tprListCommandListener**
Handles the /tpr-list slash command.
Lists all approved temporary pass requests in the channel for that week.

**tprDenyModalListener**
Handles the modal submission when an admin denies a request.
Extracts the denial reason.
DMs the user with the denial reason.
Notifies the admin and deletes the approval message.

**InitTPRScheduler**
Starts a goroutine that clears the approved TPRs and forum post message every Sunday at midnight UTC.
*/

const temporaryPassRequestCustomID = "temporary-pass-request"
const temporaryPassRequestSubmitCustomID = "temporary-pass-request-submit"
const tprApprovePrefix = "tpr-approve"
const tprDenyPrefix = "tpr-deny"

const tprApprovalChannelID = snowflake.ID(1382136230069928046)
const tprForumThreadID = snowflake.ID(1382212750289408030) // TPRs forum thread ID

//const tprForumThreadID = snowflake.ID(1385734291626922146)		// for 72nd server
//const tprApprovalChannelID = snowflake.ID(645668825668517888)		// for 72nd server

//go:embed temporary_pass_request_description.txt
var temporaryPassRequestDescription string

type TemporaryPassRequest struct {
	UserID      string
	UserName    string
	RequestedAt time.Time
	Operation   time.Time
	ThreadID    snowflake.ID
}

type ApprovedTPR struct {
	UserID      string
	Nickname    string
	UserName    string
	RequestedAt time.Time
	Operation   time.Time
	Approved    *bool
	ApprovedBy  string
	ApprovedAt  time.Time
}

var (
	approvedTPRs      []ApprovedTPR
	approvedTPRsMutex sync.Mutex
	pendingTPRs       = make(map[string]TemporaryPassRequest)

	forumMessageID snowflake.ID
	forumMutex     sync.Mutex
)

type DeniedTPR struct {
	UserID      string
	Nickname    string
	UserName    string
	RequestedAt time.Time
	Operation   time.Time
	Denied      *bool
	DeniedBy    string
	DeniedAt    time.Time
	Reason      string
}

var (
	deniedTPRs      []DeniedTPR
	deniedTPRsMutex sync.Mutex
)

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass", temporaryPassRequestCustomID),
	[]bot.EventListener{
		temporaryPassRequestEventListener,
		temporaryPassRequestSubmitEventListener,
		tprApprovalListener,
		tprListCommandListener,
		tprDenyModalListener,
	},
}

var temporaryPassRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("Temporary Pass Request").
					SetColor(0x5765f2).
					SetDescription(temporaryPassRequestDescription).
					Build()).
				AddActionRow(discord.NewPrimaryButton("Submit", temporaryPassRequestSubmitCustomID)).
				Build(),
		)
		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var temporaryPassRequestSubmitEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestSubmitCustomID {
		today := time.Now().UTC()
		today = time.Date(today.Year(), today.Month(), today.Day(), 23, 0, 0, 0, time.UTC)
		offset := (6 - int(today.Weekday()) + 7) % 7
		nextSaturday := today.AddDate(0, 0, offset)

		request := TemporaryPassRequest{
			UserID:      event.User().ID.String(),
			UserName:    event.User().Tag(),
			RequestedAt: time.Now().UTC(),
			Operation:   nextSaturday,
			// ThreadID removed since no thread created
		}

		// Send message to approval channel for admins to approve/deny the request
		_, err := event.Client().Rest().CreateMessage(
			tprApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf(
					"📝 Temporary Pass submitted by <@%s>\n• Submitted: <t:%d:F>\n• Operation: <t:%d:R>",
					request.UserID, request.RequestedAt.Unix(), request.Operation.Unix(),
				)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", tprApprovePrefix+request.UserID),
					discord.NewDangerButton("Deny", tprDenyPrefix+request.UserID),
				).
				Build(),
		)
		if err != nil {
			slog.Error("failed to post initial TPR message", slog.Any("err", err))
			return
		}

		// Save the request in pending map without ThreadID
		pendingTPRs[request.UserID] = request

		// Reply to user with ephemeral confirmation only, no thread mention
		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your request has been submitted! You will receive updates via DM.").
			Build(),
		)
	}
})

var tprApprovalListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()

	if strings.HasPrefix(customID, tprApprovePrefix) {
		userID := strings.TrimPrefix(customID, tprApprovePrefix)

		request, ok := pendingTPRs[userID]
		if !ok {
			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				SetContent("Request not found or already processed.").
				ClearContainerComponents().
				Build())
			return
		}

		delete(pendingTPRs, userID)

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		approved := ApprovedTPR{
			UserID:      request.UserID,
			Nickname:    nickname,
			UserName:    request.UserName,
			RequestedAt: request.RequestedAt,
			Operation:   request.Operation,
			ApprovedBy:  nickname,
			ApprovedAt:  time.Now().UTC(),
		}

		approvedTPRsMutex.Lock()
		for i := range approvedTPRs {
			if approvedTPRs[i].UserID == userID {
				approved := true
				approvedTPRs[i].Approved = &approved
				approvedTPRs[i].ApprovedBy = event.User().Username
				break
			}
		}
		approvedTPRs = append(approvedTPRs, approved)
		approvedTPRsMutex.Unlock()

		// Update forum summary

		forumMutex.Lock()
		defer forumMutex.Unlock()

		var contentBuilder strings.Builder
		contentBuilder.WriteString("**Temporary Pass Request Log:**")

		approvedTPRsMutex.Lock()
		for _, tpr := range approvedTPRs {
			contentBuilder.WriteString(fmt.Sprintf(
				"\n\n• TPR from **%s**\n• ✅ approved by **%s**\n• submitted at %s",
				tpr.Nickname,
				tpr.ApprovedBy,
				tpr.ApprovedAt.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST")))
		}
		approvedTPRsMutex.Unlock()

		deniedTPRsMutex.Lock()
		for _, tpr := range deniedTPRs {
			contentBuilder.WriteString(fmt.Sprintf(
				"• <@%s> denied by <@%s> at %s — reason: %s\n",
				tpr.UserID, tpr.DeniedBy, tpr.DeniedAt.Format(time.RFC1123), tpr.Reason))
		}
		deniedTPRsMutex.Unlock()

		if forumMessageID == 0 {
			msg, err := event.Client().Rest().CreateMessage(tprForumThreadID,
				discord.NewMessageCreateBuilder().SetContent(contentBuilder.String()).Build())
			if err != nil {
				slog.Error("failed to create forum summary message", slog.Any("err", err))
			} else {
				forumMessageID = msg.ID
			}
		} else {
			_, err := event.Client().Rest().UpdateMessage(tprForumThreadID, forumMessageID,
				discord.NewMessageUpdateBuilder().SetContent(contentBuilder.String()).Build())
			if err != nil {
				slog.Error("failed to update forum summary message", slog.Any("err", err))
			}
		}

		// DM the user
		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err != nil {
			slog.Error("failed to create DM channel", slog.Any("err", err))
		} else {
			expiresAt := approved.Operation.Add(24 * time.Hour)
			_, err := event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent(fmt.Sprintf(
						"✅ Your temporary pass request has been **approved**.\nIt will expire on: <t:%d:F>, <t:%d:R>",
						expiresAt.Unix(), expiresAt.Unix(),
					)).
					Build(),
			)
			if err != nil {
				slog.Error("failed to send DM to user", slog.Any("err", err))
			}
		}

		// Remove approval message
		err = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)
		if err != nil {
			slog.Error("failed to delete approval message", slog.Any("err", err))
		}

	} else if strings.HasPrefix(customID, tprDenyPrefix) {
		userID := strings.TrimPrefix(customID, tprDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny Temporary Pass").
				SetCustomID("tpr-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating deny modal", slog.Any("err", err))
		}
	}
})

var tprListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() == "tpr-list" {
		approvedTPRsMutex.Lock()
		defer approvedTPRsMutex.Unlock()
		if len(approvedTPRs) == 0 {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent("No approved temporary pass requests.").
				Build())
			return
		}
		var content strings.Builder
		content.WriteString("**Approved Temporary Pass Requests:**\n")
		for _, tpr := range approvedTPRs {
			content.WriteString(fmt.Sprintf("- <@%s> for <t:%d:R>\n", tpr.UserID, tpr.Operation.Unix()))
		}
		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(content.String()).
			Build())
	}
})

var tprDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, "tpr-deny-modal:") {
		userID := strings.TrimPrefix(event.Data.CustomID, "tpr-deny-modal:")

		request, ok := pendingTPRs[userID]
		if !ok {
			_ = event.CreateMessage(discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Request not found or already processed.").
				Build())
			return
		}
		delete(pendingTPRs, userID)

		reason, _ := event.Data.TextInputComponent("deny-reason")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}
		// Store denied TPR
		deniedTPRsMutex.Lock()
		for i := range deniedTPRs {
			if deniedTPRs[i].UserID == userID {
				approved := true
				deniedTPRs[i].Denied = &approved
				deniedTPRs[i].DeniedBy = event.User().Username
				break
			}
		}
		deniedTPRs = append(deniedTPRs, DeniedTPR{
			UserID:      request.UserID,
			UserName:    request.UserName,
			Nickname:    nickname,
			RequestedAt: request.RequestedAt,
			Operation:   request.Operation,
			DeniedBy:    event.User().Tag(),
			DeniedAt:    time.Now().UTC(),
			Reason:      reason.Value,
		})
		deniedTPRsMutex.Unlock()

		// Notify user via DM
		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, err := event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your temporary pass request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
			if err != nil {
				slog.Error("failed to send DM to user", slog.Any("err", err))
			}
		} else {
			slog.Error("failed to create DM channel", slog.Any("err", err))
		}

		// Acknowledge to admin and delete original message
		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("Temporary pass request denied and user notified.").
			SetEphemeral(true).
			Build())

		forumMutex.Lock()
		defer forumMutex.Unlock()

		var contentBuilder strings.Builder
		contentBuilder.WriteString("**Temporary Pass Request Log:**")

		approvedTPRsMutex.Lock()
		for _, tpr := range approvedTPRs {
			contentBuilder.WriteString(fmt.Sprintf(
				"\n\n• TPR from **%s**\n• ✅ approved by **%s**\n• submitted at %s",
				tpr.Nickname,
				tpr.ApprovedBy,
				tpr.ApprovedAt.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST")))
		}
		approvedTPRsMutex.Unlock()

		deniedTPRsMutex.Lock()
		for _, tpr := range deniedTPRs {
			contentBuilder.WriteString(fmt.Sprintf(
				"\n\n• TPR from **%s**\n• ❌ denied by **%s**\n• submitted at %s\n• reason: **%s**",
				tpr.Nickname,
				tpr.DeniedBy,
				tpr.DeniedAt.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
				tpr.Reason))
		}
		deniedTPRsMutex.Unlock()

		if forumMessageID == 0 {
			msg, err := event.Client().Rest().CreateMessage(tprForumThreadID,
				discord.NewMessageCreateBuilder().SetContent(contentBuilder.String()).Build())
			if err != nil {
				slog.Error("failed to create forum summary message", slog.Any("err", err))
			} else {
				forumMessageID = msg.ID
			}
		} else {
			_, err := event.Client().Rest().UpdateMessage(tprForumThreadID, forumMessageID,
				discord.NewMessageUpdateBuilder().SetContent(contentBuilder.String()).Build())
			if err != nil {
				slog.Error("failed to update forum summary message", slog.Any("err", err))
			}
		}

		_ = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)
	}
})

func InitTPRScheduler(client bot.Client) {
	go func() {
		for {
			now := time.Now().UTC()
			daysUntilSunday := (7 - int(now.Weekday())) % 7
			nextSunday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, daysUntilSunday)
			duration := nextSunday.Sub(now)
			if duration <= 0 {
				// If duration is zero or negative, sleep for 1 second to avoid tight loop
				time.Sleep(time.Second)
				continue
			}
			time.Sleep(duration)

			approvedTPRsMutex.Lock()
			approvedTPRs = nil
			approvedTPRsMutex.Unlock()
			forumMutex.Lock()
			forumMessageID = 0
			forumMutex.Unlock()
			slog.Info("Cleared approved TPRs and forum post for new week")
		}
	}()
}

// TODO: add function to auto populate campaign attendance google sheet with approved TPRs after each approval
//		 OR fill text file with approved TPRs one per line via discord nickname i.e. SFC A. Hydra and to be used by prodigy's script (probably easier to do this way)
