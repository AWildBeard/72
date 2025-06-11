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

const temporaryPassRequestCustomID = "temporary-pass-request"
const temporaryPassRequestSubmitCustomID = "temporary-pass-request-submit"
const tprApprovePrefix = "tpr-approve"
const tprDenyPrefix = "tpr-deny"

const tprApprovalChannelID = snowflake.ID(1382136230069928046)
const tprForumThreadID = snowflake.ID(1382212750289408030) // TPRs forum thread ID

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
	UserName    string
	RequestedAt time.Time
	Operation   time.Time
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

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass", temporaryPassRequestCustomID),
	[]bot.EventListener{
		temporaryPassRequestEventListener,
		temporaryPassRequestSubmitEventListener,
		tprApprovalListener,
		tprListCommandListener,
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

	if strings.HasPrefix(customID, tprApprovePrefix) || strings.HasPrefix(customID, tprDenyPrefix) {
		isApproval := strings.HasPrefix(customID, tprApprovePrefix)
		userID := strings.TrimPrefix(customID, tprApprovePrefix)
		if !isApproval {
			userID = strings.TrimPrefix(customID, tprDenyPrefix)
		}

		request, ok := pendingTPRs[userID]
		if !ok {
			_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				SetContent("Request not found or already processed.").
				ClearContainerComponents().
				Build())
			return
		}

		delete(pendingTPRs, userID)

		statusText := "denied"
		if isApproval {
			approved := ApprovedTPR{
				UserID:      request.UserID,
				UserName:    request.UserName,
				RequestedAt: request.RequestedAt,
				Operation:   request.Operation,
				ApprovedBy:  event.User().Tag(),
				ApprovedAt:  time.Now().UTC(),
			}

			approvedTPRsMutex.Lock()
			approvedTPRs = append(approvedTPRs, approved)
			approvedTPRsMutex.Unlock()
			statusText = "approved"
		}

		// Update forum summary if approved
		if isApproval {
			forumMutex.Lock()
			defer forumMutex.Unlock()

			var contentBuilder strings.Builder
			contentBuilder.WriteString("**Approved Temporary Pass Requests:**\n")

			approvedTPRsMutex.Lock()
			for _, tpr := range approvedTPRs {
				contentBuilder.WriteString(fmt.Sprintf(
					"• <@%s> approved by <@%s> at %s\n", tpr.UserID, tpr.ApprovedBy, tpr.ApprovedAt.Format(time.RFC1123)))
			}
			approvedTPRsMutex.Unlock()

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
		}

		// Send DM to the user with status (no buttons)
		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err != nil {
			slog.Error("failed to create DM channel", slog.Any("err", err))
		} else {
			_, err := event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("Your temporary pass request has been **%s**.", statusText).
					Build(),
			)
			if err != nil {
				slog.Error("failed to send DM to user", slog.Any("err", err))
			}
		}

		// Delete the original message in the approval channel
		err = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)
		if err != nil {
			slog.Error("failed to delete approval message", slog.Any("err", err))
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

func InitTPRScheduler(client bot.Client) {
	go func() {
		for {
			now := time.Now().UTC()
			daysUntilSunday := (7 - int(now.Weekday())) % 7
			nextSunday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, daysUntilSunday)
			duration := nextSunday.Sub(now)
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
