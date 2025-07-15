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

const squadXMLCustomID = "squad-xml-button"
const squadXMLCreateModalCustomID = "squad-xml-modal"
const squadXMLSubmitModalCustomID = "squad-xml-modal-submit"

//const xmlForumThreadID = snowflake.ID(1384022967645769838)
//const xmlApprovalChannelID = snowflake.ID(1382136230069928046)

const xmlForumThreadID = snowflake.ID(1385734680661201018)    // for 72nd server
const xmlApprovalChannelID = snowflake.ID(645668825668517888) // for 72nd server

const xmlApprovePrefix = "xml-approve"
const xmlDenyPrefix = "xml-deny"
const xmlDenyModalPrefix = "xml-deny-modal:"

//go:embed squad_xml_description.txt
var squadXMLDescription string

type SquadXML struct {
	UserID     string
	PlayerName string
	PlayerID   string
	Nickname   string
	Username   string
	Submitted  time.Time
	Approved   *bool
	ReviewedBy string
	DenyReason string
}

var (
	xmlRequests        []SquadXML
	xmlRequestsMutex   sync.Mutex
	xmlForumMessageID  snowflake.ID
	xmlForumAdded      = make(map[string]bool)
	xmlForumAddedMutex sync.Mutex
)

var squadXML = ButtonEventHandler{
	discord.NewPrimaryButton("Squad XML", squadXMLCustomID),
	[]bot.EventListener{
		squadXMLEventListener,
		squadXMLModalRequestEventListener,
		squadXMLModalSubmitEventListener,
		xmlModalSubmissionEventListener,
		xmlApprovalButtonListener,
		xmlDenyModalListener,
	},
}

var squadXMLEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == squadXMLCustomID {
		//TODO: Submit the squad XML request in the S1 Forum

		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("Squad XML Request Instructions").
					SetDescription(squadXMLDescription).
					SetColor(0x5765f2).
					Build(),
				).
				AddActionRow(discord.NewPrimaryButton("Add Details & Submit", squadXMLCreateModalCustomID)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var squadXMLModalRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == squadXMLCreateModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Squad XML Request").
				SetCustomID(squadXMLSubmitModalCustomID).
				AddActionRow(discord.NewShortTextInput("name", "Player Name")).
				AddActionRow(discord.NewShortTextInput("player_id", "Player ID")).
				Build())

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var squadXMLModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == squadXMLSubmitModalCustomID {
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("✅ Your Squad XML request has been submitted. You will receive updates via DM.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var xmlModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.Data.CustomID == squadXMLSubmitModalCustomID {
		PlayerName, _ := event.Data.TextInputComponent("name")
		PlayerID, _ := event.Data.TextInputComponent("player_id")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		xml := SquadXML{
			UserID:     event.User().ID.String(),
			PlayerName: PlayerName.Value,
			PlayerID:   PlayerID.Value,
			Username:   event.User().Tag(),
			Nickname:   nickname,
			Submitted:  time.Now().UTC(),
		}

		xmlRequestsMutex.Lock()
		xmlRequests = append(xmlRequests, xml)
		xmlRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			xmlApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Squad XML request submitted by **%s**\n• Player Name: **%s** Player ID: **%s**",
					xml.Nickname, xml.PlayerName, xml.PlayerID)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", xmlApprovePrefix+xml.UserID),
					discord.NewDangerButton("Deny", xmlDenyPrefix+xml.UserID),
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
					SetContent("✅ Your Squad XML request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your Squad XML request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var xmlApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, xmlApprovePrefix) {
		userID := strings.TrimPrefix(customID, xmlApprovePrefix)

		xmlRequestsMutex.Lock()
		for i := range xmlRequests {
			if xmlRequests[i].UserID == userID {
				approved := true
				xmlRequests[i].Approved = &approved
				xmlRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		xmlRequestsMutex.Unlock()

		updatexmlForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your Squad XML request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(xmlApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, xmlDenyPrefix) {
		userID := strings.TrimPrefix(customID, xmlDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny Squad XML Request").
				SetCustomID("xml-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var xmlDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, xmlDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, xmlDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		xmlRequestsMutex.Lock()
		for i := range xmlRequests {
			if xmlRequests[i].UserID == userID {
				approved := false
				xmlRequests[i].Approved = &approved
				xmlRequests[i].ReviewedBy = event.User().Tag()
				xmlRequests[i].DenyReason = reason.Value
				break
			}
		}
		xmlRequestsMutex.Unlock()

		updatexmlForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your Squad XML request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("Squad XML request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(xmlApprovalChannelID, event.Message.ID)
	}
})

func updatexmlForumPost(client bot.Client) {
	xmlRequestsMutex.Lock()
	defer xmlRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range xmlRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.PlayerID, req.Approved)

		xmlForumAddedMutex.Lock()
		if xmlForumAdded[key] {
			xmlForumAddedMutex.Unlock()
			continue
		}
		xmlForumAdded[key] = true
		xmlForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = "✅ approved by " + req.ReviewedBy
		} else {
			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n• Squad XML submitted by **%s** at %s with Player ID of: **%s** (Status: %s)\n",
			req.PlayerName,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.PlayerID,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if xmlForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(xmlForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Squad XML Log:**\n"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create xml forum post", slog.Any("err", err))
			return
		}
		xmlForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(xmlForumThreadID, xmlForumMessageID)
		if err != nil {
			slog.Error("failed to fetch xml forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(xmlForumThreadID, xmlForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update xml forum post", slog.Any("err", err))
		}
	}
}
