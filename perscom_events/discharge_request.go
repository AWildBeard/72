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

const dischargeRequestCustomID = "discharge-request"
const dischargeRequestStatementModalCustomID = "discharge-request-statement-modal"
const dischargeRequestStatementSubmitModalCustomID = "discharge-request-statement-modal-submit"

const disForumThreadID = snowflake.ID(1385484591200211157)
const disApprovalChannelID = snowflake.ID(1382136230069928046)
const disApprovePrefix = "dis-approve"
const disDenyPrefix = "dis-deny"
const disDenyModalPrefix = "dis-deny-modal:"

//go:embed discharge_request_description.txt
var dischargeRequestDescription string

type Discharge struct {
	UserID     string
	Statement  string
	Nickname   string
	Username   string
	Submitted  time.Time
	Approved   *bool
	ReviewedBy string
	DenyReason string
}

var (
	disRequests        []Discharge
	disRequestsMutex   sync.Mutex
	disForumMessageID  snowflake.ID
	disForumAdded      = make(map[string]bool)
	disForumAddedMutex sync.Mutex
)

var dischargeRequest = ButtonEventHandler{
	discord.NewDangerButton("Discharge", dischargeRequestCustomID),
	[]bot.EventListener{
		dischargeRequestEventHandler,
		dischargeRequestStatementModalEventListener,
		dischargeRequestStatementModalSubmitEventListener,
		disModalSubmissionEventListener,
		disApprovalButtonListener,
		disDenyModalListener,
	},
}

var dischargeRequestEventHandler = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == dischargeRequestCustomID {

		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Discharge Request").
				SetColor(0xFF0000).
				SetDescription(dischargeRequestDescription).
				Build(),
			).
			AddActionRow(
				discord.NewPrimaryButton("Add Details & Submit", dischargeRequestStatementModalCustomID),
			).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var dischargeRequestStatementModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == dischargeRequestStatementModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Discharge Request Statement").
				SetCustomID(dischargeRequestStatementSubmitModalCustomID).
				AddActionRow(discord.NewShortTextInput("statement", "Statement")).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var dischargeRequestStatementModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == dischargeRequestStatementSubmitModalCustomID {
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("✅ Your discharge request has been submitted. You will receive updates via DM.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var disModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.Data.CustomID == dischargeRequestStatementSubmitModalCustomID {

		statement, _ := event.Data.TextInputComponent("statement")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		dis := Discharge{
			UserID:    event.User().ID.String(),
			Statement: statement.Value,
			Username:  event.User().Tag(),
			Nickname:  nickname,
			Submitted: time.Now().UTC(),
		}

		disRequestsMutex.Lock()
		disRequests = append(disRequests, dis)
		disRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			disApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("**Discharge** request submitted by **%s**\n• Statement: **%s**",
					dis.Nickname, dis.Statement)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", disApprovePrefix+dis.UserID),
					discord.NewDangerButton("Deny", disDenyPrefix+dis.UserID),
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
					SetContent("✅ Your discharge request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your discharge request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var disApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, disApprovePrefix) {
		userID := strings.TrimPrefix(customID, disApprovePrefix)

		disRequestsMutex.Lock()
		for i := range disRequests {
			if disRequests[i].UserID == userID {
				approved := true
				disRequests[i].Approved = &approved
				disRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		disRequestsMutex.Unlock()

		updatedisForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your discharge request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(disApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, disDenyPrefix) {
		userID := strings.TrimPrefix(customID, disDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny discharge Request").
				SetCustomID("dis-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var disDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, disDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, disDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		disRequestsMutex.Lock()
		for i := range disRequests {
			if disRequests[i].UserID == userID {
				approved := false
				disRequests[i].Approved = &approved
				disRequests[i].ReviewedBy = event.User().Tag()
				disRequests[i].DenyReason = reason.Value
				break
			}
		}
		disRequestsMutex.Unlock()

		updatedisForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your discharge request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("discharge request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(disApprovalChannelID, event.Message.ID)
	}
})

func updatedisForumPost(client bot.Client) {
	disRequestsMutex.Lock()
	defer disRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range disRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Statement, req.Approved)

		disForumAddedMutex.Lock()
		if disForumAdded[key] {
			disForumAddedMutex.Unlock()
			continue
		}
		disForumAdded[key] = true
		disForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = fmt.Sprintf("✅ approved by: **%s**", req.Nickname)
		} else {
			status = fmt.Sprintf("❌ denied by: **%s**. Reason: **%s**", req.Nickname, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n\n• **Discharge** request submitted by **%s** at %s. \n• statement: **%s**\n• Status: %s\n",
			req.Nickname,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.Statement,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if disForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(disForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Discharge Log:**"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create dis forum post", slog.Any("err", err))
			return
		}
		disForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(disForumThreadID, disForumMessageID)
		if err != nil {
			slog.Error("failed to fetch dis forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(disForumThreadID, disForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update dis forum post", slog.Any("err", err))
		}
	}
}
