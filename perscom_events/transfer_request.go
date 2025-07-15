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

const transferRequestCustomID = "transfer-request"
const transferRequestModalCustomID = "transfer-request-modal"
const transferRequestModalSubmitCustomID = "transfer-request-modal-submit"

//const transForumThreadID = snowflake.ID(1384355420356739143)
//const transApprovalChannelID = snowflake.ID(1382136230069928046)

const transForumThreadID = snowflake.ID(1385734619579678740)    // for 72nd server
const transApprovalChannelID = snowflake.ID(645668825668517888) // for 72nd server

const transApprovePrefix = "trans-approve"
const transDenyPrefix = "trans-deny"
const transDenyModalPrefix = "trans-deny-modal:"

//go:embed transfer_request_description.txt
var transferRequestDescription string

type Transfer struct {
	UserID     string
	From       string
	To         string
	Nickname   string
	Username   string
	Submitted  time.Time
	Approved   *bool
	ReviewedBy string
	DenyReason string
}

var (
	transRequests        []Transfer
	transRequestsMutex   sync.Mutex
	transForumMessageID  snowflake.ID
	transForumAdded      = make(map[string]bool)
	transForumAddedMutex sync.Mutex
)

var transferRequest = ButtonEventHandler{
	Button: discord.NewPrimaryButton("Transfer", transferRequestCustomID),
	EventListeners: []bot.EventListener{
		transferRequestEventListener,
		transferRequestModalEventListener,
		transferRequestModalSubmitEventListener,
		transModalSubmissionEventListener,
		transApprovalButtonListener,
		transDenyModalListener,
	},
}

var transferRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == transferRequestCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Transfer Request").
				SetColor(0x5765f2).
				SetDescription(transferRequestDescription).
				Build(),
			).
			AddActionRow(discord.NewPrimaryButton("Add Details & Submit", transferRequestModalCustomID)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var transferRequestModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == transferRequestModalCustomID {
		err := event.Modal(discord.NewModalCreateBuilder().
			SetTitle("Unit Transfer Request").
			SetCustomID(transferRequestModalSubmitCustomID).
			AddActionRow(discord.NewShortTextInput("from", "Current Unit")).
			AddActionRow(discord.NewShortTextInput("to", "Desired Unit")).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating modal", slog.Any("err", err))
		}
	}
})

var transferRequestModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == transferRequestModalSubmitCustomID {
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("✅ Your transfer request has been submitted. You will receive updates via DM.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var transModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.Data.CustomID == transferRequestModalSubmitCustomID {

		From, _ := event.Data.TextInputComponent("from")
		To, _ := event.Data.TextInputComponent("to")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		trans := Transfer{
			UserID:    event.User().ID.String(),
			From:      From.Value,
			To:        To.Value,
			Username:  event.User().Tag(),
			Nickname:  nickname,
			Submitted: time.Now().UTC(),
		}

		transRequestsMutex.Lock()
		transRequests = append(transRequests, trans)
		transRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			transApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Transfer request submitted by **%s** \n• Current Unit: **%s** \n• Desired Unit: **%s**",
					trans.Nickname, trans.From, trans.To)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", transApprovePrefix+trans.UserID),
					discord.NewDangerButton("Deny", transDenyPrefix+trans.UserID),
				).
				Build(),
		)
		if err != nil {
			slog.Error("error while creating approval message", slog.Any("err", err))
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(event.User().ID)
		if err != nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your transfer request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your transfer request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var transApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, transApprovePrefix) {
		userID := strings.TrimPrefix(customID, transApprovePrefix)

		transRequestsMutex.Lock()
		for i := range transRequests {
			if transRequests[i].UserID == userID {
				approved := true
				transRequests[i].Approved = &approved
				transRequests[i].ReviewedBy = event.User().Username
				break
			}
		}
		transRequestsMutex.Unlock()

		updatetransForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your transfer request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(transApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, transDenyPrefix) {
		userID := strings.TrimPrefix(customID, transDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny transfer Request").
				SetCustomID("trans-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var transDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, transDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, transDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		transRequestsMutex.Lock()
		for i := range transRequests {
			if transRequests[i].UserID == userID {
				approved := false
				transRequests[i].Approved = &approved
				transRequests[i].ReviewedBy = event.User().Tag()
				transRequests[i].DenyReason = reason.Value
				break
			}
		}
		transRequestsMutex.Unlock()

		updatetransForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your transfer request has been **denied**. **Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("transfer request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(transApprovalChannelID, event.Message.ID)
	}
})

func updatetransForumPost(client bot.Client) {
	transRequestsMutex.Lock()
	defer transRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range transRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.To, req.Approved)

		transForumAddedMutex.Lock()
		if transForumAdded[key] {
			transForumAddedMutex.Unlock()
			continue
		}
		transForumAdded[key] = true
		transForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = fmt.Sprintf("✅ approved by: **%s**", req.Nickname)
		} else {
			status = fmt.Sprintf("❌ denied by: **%s**. Reason: **%s**", req.Nickname, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n\n• Transfer request submitted by **%s** at %s. \n• Current Unit: **%s** \n• Desired Unit: **%s**\n• Status: %s\n",
			req.Nickname,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.From,
			req.To,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if transForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(transForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Transfer Log:**"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create trans forum post", slog.Any("err", err))
			return
		}
		transForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(transForumThreadID, transForumMessageID)
		if err != nil {
			slog.Error("failed to fetch trans forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(transForumThreadID, transForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update trans forum post", slog.Any("err", err))
		}
	}
}
