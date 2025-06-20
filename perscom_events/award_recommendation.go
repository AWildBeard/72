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

const awardRecommendationCustomID = "award-recommendation"
const awardRecommendationModalCustomID = "award-recommendation-modal"
const awardRecommendationModalSubmitCustomID = "award-recommendation-modal-submit"

const awdForumThreadID = snowflake.ID(1385471130894209144)
const awdApprovalChannelID = snowflake.ID(1382136230069928046)

//const awdForumThreadID = snowflake.ID(1385734844121612308)		// for 72nd server
//const awdApprovalChannelID = snowflake.ID(645668825668517888)		// for 72nd server

const awdApprovePrefix = "awd-approve"
const awdDenyPrefix = "awd-deny"
const awdDenyModalPrefix = "awd-deny-modal:"

//go:embed award_recommendation_description.txt
var awardRecommendationDescription string

type Award struct {
	UserID     string
	awardName  string
	Recipient  string
	OpNumber   string
	Citation   string
	Nickname   string
	Username   string
	Submitted  time.Time
	Approved   *bool
	ReviewedBy string
	DenyReason string
}

var (
	awdRequests        []Award
	awdRequestsMutex   sync.Mutex
	awdForumMessageID  snowflake.ID
	awdForumAdded      = make(map[string]bool)
	awdForumAddedMutex sync.Mutex
)

var awardRecommendation = ButtonEventHandler{
	Button: discord.NewPrimaryButton("Award Rec", awardRecommendationCustomID),
	EventListeners: []bot.EventListener{
		awardRecommendationEventListener,
		awardRecommendationModalEventListener,
		awardRecommendationModalSubmitEventListener,
		awdModalSubmissionEventListener,
		awdApprovalButtonListener,
		awdDenyModalListener,
	},
}

var awardRecommendationEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == awardRecommendationCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle(":military_medal: Award Recommendation :military_medal:").
				SetColor(0x5765f2).
				SetDescription(awardRecommendationDescription).
				Build(),
			).
			AddActionRow(discord.NewStringSelectMenu(awardRecommendationModalCustomID, "Select an option...",
				discord.NewStringSelectMenuOption("Air Service Medal", "Air Service Medal"),
				discord.NewStringSelectMenuOption("Army Achievement Medal", "Army Achievement Medal"),
				discord.NewStringSelectMenuOption("Army Commendation Medal", "Army Commendation Medal"),
				discord.NewStringSelectMenuOption("Army NCODEV Ribbon", "Army NCODEV Ribbon"),
				discord.NewStringSelectMenuOption("Bronze Star Medal", "Bronze Star Medal"),
			)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var awardRecommendationModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == awardRecommendationModalCustomID {
		selectedOption := event.StringSelectMenuInteractionData().Values[0]

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Award Recommendation").
				SetCustomID(awardRecommendationModalSubmitCustomID + ":" + selectedOption).
				AddActionRow(discord.NewShortTextInput("name", "Recipient Name")).
				AddActionRow(discord.NewShortTextInput("operation_number", "Operation #")).
				AddActionRow(discord.NewParagraphTextInput("citation", "Citation")).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var awardRecommendationModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.Contains(event.ModalSubmitInteraction.Data.CustomID, awardRecommendationModalSubmitCustomID) {
		selectedOption := strings.Split(event.ModalSubmitInteraction.Data.CustomID, ":")[1]

		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContentf("✅ Submitted your award recommendation request for \"%v\". You will receive updates via DM.", selectedOption).
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var awdModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, awardRecommendationModalSubmitCustomID+":") {
		parts := strings.SplitN(event.Data.CustomID, ":", 2)
		award := ""
		if len(parts) == 2 {
			award = parts[1]
		}

		recipient, _ := event.Data.TextInputComponent("name")
		opNumber, _ := event.Data.TextInputComponent("operation_number")
		citation, _ := event.Data.TextInputComponent("citation")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		awd := Award{
			UserID:    event.User().ID.String(),
			awardName: award,
			Recipient: recipient.Value,
			OpNumber:  opNumber.Value,
			Citation:  citation.Value,
			Username:  event.User().Tag(),
			Nickname:  nickname,
			Submitted: time.Now().UTC(),
		}

		awdRequestsMutex.Lock()
		awdRequests = append(awdRequests, awd)
		awdRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			awdApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("**%s** award recomendation submitted by **%s**\n• Recipient Name: **%s**\n• Operation #: **%s**\n• Citation: **%s**",
					awd.awardName, awd.Nickname, awd.Recipient, awd.OpNumber, awd.Citation)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", awdApprovePrefix+awd.UserID),
					discord.NewDangerButton("Deny", awdDenyPrefix+awd.UserID),
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
					SetContent("✅ Your award request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your award request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var awdApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, awdApprovePrefix) {
		userID := strings.TrimPrefix(customID, awdApprovePrefix)

		awdRequestsMutex.Lock()
		for i := range awdRequests {
			if awdRequests[i].UserID == userID {
				approved := true
				awdRequests[i].Approved = &approved
				awdRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		awdRequestsMutex.Unlock()

		updateawdForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your award request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(awdApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, awdDenyPrefix) {
		userID := strings.TrimPrefix(customID, awdDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny award Request").
				SetCustomID("awd-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var awdDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, awdDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, awdDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		awdRequestsMutex.Lock()
		for i := range awdRequests {
			if awdRequests[i].UserID == userID {
				approved := false
				awdRequests[i].Approved = &approved
				awdRequests[i].ReviewedBy = event.User().Tag()
				awdRequests[i].DenyReason = reason.Value
				break
			}
		}
		awdRequestsMutex.Unlock()

		updateawdForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your award request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("award request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(awdApprovalChannelID, event.Message.ID)
	}
})

func updateawdForumPost(client bot.Client) {
	awdRequestsMutex.Lock()
	defer awdRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range awdRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Recipient, req.Approved)

		awdForumAddedMutex.Lock()
		if awdForumAdded[key] {
			awdForumAddedMutex.Unlock()
			continue
		}
		awdForumAdded[key] = true
		awdForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = fmt.Sprintf("✅ approved by: **%s**", req.Nickname)
		} else {
			status = fmt.Sprintf("❌ denied by: **%s**. Reason: **%s**", req.Nickname, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n\n• %s submitted by **%s** at %s.\n• Recipient: **%s**\n• Operation Number: **%s**\n• Citation: **%s**\n• Status: %s\n",
			req.awardName,
			req.Nickname,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.Recipient,
			req.OpNumber,
			req.Citation,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if awdForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(awdForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Award Log:**"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create awd forum post", slog.Any("err", err))
			return
		}
		awdForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(awdForumThreadID, awdForumMessageID)
		if err != nil {
			slog.Error("failed to fetch awd forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(awdForumThreadID, awdForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update awd forum post", slog.Any("err", err))
		}
	}
}
