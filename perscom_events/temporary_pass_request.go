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

const (
	temporaryPassRequestCustomID       = "temporary-pass-request"
	temporaryPassRequestSubmitCustomID = "temporary-pass-request-submit"
	//tprForumThreadID                   = snowflake.ID(1382212750289408030) // testing thread
	//tprApprovalChannelID               = snowflake.ID(1382136230069928046) // testing channel
	tprForumThreadID     = snowflake.ID(1385734291626922146) // 72nd thread
	tprApprovalChannelID = snowflake.ID(645668825668517888)  // 72nd channel
	tprApprovePrefix     = "tpr-approve"
	tprDenyPrefix        = "tpr-deny"
	tprDenyModalPrefix   = "tpr-deny-modal:"
)

//go:embed temporary_pass_request_description.txt
var temporaryPassRequestDescription string

type TemporaryPassRequest struct {
	UserID       string
	UserName     string
	Nickname     string
	RequestedAt  time.Time
	Operation    time.Time
	Approved     *bool
	ReviewedBy   string
	DeniedReason string
}

var (
	tprRequests        []TemporaryPassRequest
	tprRequestsMutex   sync.Mutex
	tprForumMessageID  snowflake.ID
	tprForumAdded      = make(map[string]bool)
	tprForumAddedMutex sync.Mutex
)

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass", temporaryPassRequestCustomID),
	[]bot.EventListener{
		temporaryPassRequestEventListener,
		temporaryPassRequestSubmitEventListener,
		tprApprovalButtonListener,
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
			slog.Error("error while creating TPR message", slog.Any("err", err))
		}
	}
})

var temporaryPassRequestSubmitEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestSubmitCustomID {
		today := time.Now().UTC()
		today = time.Date(today.Year(), today.Month(), today.Day(), 23, 0, 0, 0, time.UTC)
		offset := (6 - int(today.Weekday()) + 7) % 7
		nextSaturday := today.AddDate(0, 0, offset)

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		tpr := TemporaryPassRequest{
			UserID:      event.User().ID.String(),
			UserName:    event.User().Tag(),
			Nickname:    nickname,
			RequestedAt: time.Now().UTC(),
			Operation:   nextSaturday,
		}

		tprRequestsMutex.Lock()
		tprRequests = append(tprRequests, tpr)
		tprRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			tprApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf(
					"📝 Temporary Pass submitted by <@%s>\n• Operation: <t:%d:F>",
					tpr.UserID, tpr.Operation.Unix(),
				)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", tprApprovePrefix+tpr.UserID),
					discord.NewDangerButton("Deny", tprDenyPrefix+tpr.UserID),
				).
				Build(),
		)
		if err != nil {
			slog.Error("failed to post TPR approval message", slog.Any("err", err))
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your temporary pass request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var tprApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, tprApprovePrefix) {
		userID := strings.TrimPrefix(customID, tprApprovePrefix)
		tprRequestsMutex.Lock()
		for i := range tprRequests {
			if tprRequests[i].UserID == userID {
				approved := true
				tprRequests[i].Approved = &approved
				tprRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		tprRequestsMutex.Unlock()
		updateTPRForumPost(event.Client())
		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your temporary pass request has been **approved**.").
					Build(),
			)
		}
		_ = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)
	} else if strings.HasPrefix(customID, tprDenyPrefix) {
		userID := strings.TrimPrefix(customID, tprDenyPrefix)
		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny Temporary Pass").
				SetCustomID(tprDenyModalPrefix + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var tprDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, tprDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, tprDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")
		tprRequestsMutex.Lock()
		for i := range tprRequests {
			if tprRequests[i].UserID == userID {
				approved := false
				tprRequests[i].Approved = &approved
				tprRequests[i].ReviewedBy = event.User().Tag()
				tprRequests[i].DeniedReason = reason.Value
				break
			}
		}
		tprRequestsMutex.Unlock()
		updateTPRForumPost(event.Client())
		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your temporary pass request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}
		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Temporary pass request denied and user notified.").
			Build())
		_ = event.Client().Rest().DeleteMessage(tprApprovalChannelID, event.Message.ID)
	}
})

var tprListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() == "tpr-list" {
		tprRequestsMutex.Lock()
		defer tprRequestsMutex.Unlock()
		if len(tprRequests) == 0 {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent("No approved temporary pass requests.").
				Build())
			return
		}
		var content strings.Builder
		content.WriteString("**Approved Temporary Pass Requests:**\n")
		for _, tpr := range tprRequests {
			if tpr.Approved != nil && *tpr.Approved {
				content.WriteString(fmt.Sprintf("- <@%s> for <t:%d:R>\n", tpr.UserID, tpr.Operation.Unix()))
			}
		}
		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(content.String()).
			Build())
	}
})

func updateTPRForumPost(client bot.Client) {
	tprRequestsMutex.Lock()
	defer tprRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range tprRequests {
		if req.Approved == nil {
			continue
		}
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Operation.Format("2006-01-02"), req.Approved)
		tprForumAddedMutex.Lock()
		if tprForumAdded[key] {
			tprForumAddedMutex.Unlock()
			continue
		}
		tprForumAdded[key] = true
		tprForumAddedMutex.Unlock()
		status := "🕐 pending"
		if *req.Approved {
			status = "✅ approved by " + req.ReviewedBy
		} else {
			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DeniedReason)
		}
		newEntries.WriteString(fmt.Sprintf(
			"\n• %s submitted TPR at %s for: <t:%d:F> (Status: %s)\n",
			req.Nickname,
			req.RequestedAt.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.Operation.Unix(),
			status,
		))
	}

	if newEntries.Len() == 0 {
		return
	}

	if tprForumMessageID == 0 {
		msg, err := client.Rest().CreateMessage(tprForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Temporary Pass Request Log:**\n"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create TPR forum post", slog.Any("err", err))
			return
		}
		tprForumMessageID = msg.ID
	} else {
		msg, err := client.Rest().GetMessage(tprForumThreadID, tprForumMessageID)
		if err != nil {
			slog.Error("failed to fetch TPR forum post", slog.Any("err", err))
			return
		}
		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000]
		}
		_, err = client.Rest().UpdateMessage(tprForumThreadID, tprForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update TPR forum post", slog.Any("err", err))
		}
	}
}

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

			tprRequestsMutex.Lock()
			tprRequests = nil
			tprRequestsMutex.Unlock()
			tprForumMessageID = 0
			slog.Info("Cleared TPRs and forum post for new week")
		}
	}()
}
