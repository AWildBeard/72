package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
	"strings"
)

const awardRecommendationCustomID = "award-recommendation"
const awardRecommendationModalCustomID = "award-recommendation-modal"
const awardRecommendationModalSubmitCustomID = "award-recommendation-modal-submit"

//go:embed award_recommendation_description.txt
var awardRecommendationDescription string

var awardRecommendation = ButtonEventHandler{
	Button:         discord.NewPrimaryButton("Award Rec", awardRecommendationCustomID),
	EventListeners: []bot.EventListener{awardRecommendationEventListener, awardRecommendationModalEventListener, awardRecommendationModalSubmitEventListener},
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

		//TODO: Create channel for collab on the medal

		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContentf("Submitted your award recommendation request for \"%v\".", selectedOption).
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
