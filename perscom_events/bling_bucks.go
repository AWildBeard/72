package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
	"strings"
)

const blingBucksCustomID = "bling-bucks"
const selectedBBOptionCustomID = "selected-bb-option"
const blingBucksModalSubmitCustomID = "bling-bucks-modal-submit"

//go:embed bling_bucks_description.txt
var blingBucksDescription string

var blingBucksRequest = ButtonEventHandler{
	Button:         discord.NewPrimaryButton("Bling Bucks", blingBucksCustomID),
	EventListeners: []bot.EventListener{blingBucksEventListener, blingBucksSelectedOptionEventListener, blingBucksModalSubmitEventListener},
}

var blingBucksEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == blingBucksCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetColor(0xe8b923).
				SetTitle(":coin: Bling Bucks Request :coin:").
				SetDescription(blingBucksDescription).
				Build(),
			).
			AddActionRow(discord.NewStringSelectMenu(selectedBBOptionCustomID, "Select an option...",
				discord.NewStringSelectMenuOption("Raffle Ticket - 2 BB", "Raffle Ticket"),
				discord.NewStringSelectMenuOption("Helmet - 8 BB", "Helmet"),
				discord.NewStringSelectMenuOption("Insignia - 10 BB", "Insignia"),
				discord.NewStringSelectMenuOption("Uniform - 10 BB", "Uniform"),
				discord.NewStringSelectMenuOption("Backpack - 10 BB", "Backpack"),
				discord.NewStringSelectMenuOption("Vest - 12 BB", "Vest"),
				discord.NewStringSelectMenuOption("Face-wear - 16 BB", "Face-wear"),
			)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var blingBucksSelectedOptionEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == selectedBBOptionCustomID {
		var err error
		selectedOption := event.StringSelectMenuInteractionData().Values[0]

		if selectedOption == "Raffle Ticket" {
			//TODO: Make it S-1's problem. No channel necessary

			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent("Submitted your Bling Bucks request.").
				Build(),
			)
		} else {
			err = event.Modal(discord.NewModalCreateBuilder().
				SetTitle("Bling Bucks Request").
				SetCustomID(blingBucksModalSubmitCustomID + ":" + selectedOption).
				AddActionRow(discord.NewShortTextInput("name", "Name")).
				AddActionRow(discord.NewShortTextInput("player_id", "Player ID")).
				AddActionRow(discord.NewParagraphTextInput("description", "Link and/or Description")).
				Build(),
			)
		}

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var blingBucksModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.Contains(event.ModalSubmitInteraction.Data.CustomID, blingBucksModalSubmitCustomID) {
		var err error
		selectedBBOption := ""
		if split := strings.Split(event.ModalSubmitInteraction.Data.CustomID, ":"); len(split) == 2 {
			selectedBBOption = split[1]
		} else {
			slog.Error("error while splitting custom ID")
		}

		//ToDo: Create channel with details of BB request
		err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("Submitted your Bling Bucks request.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err), slog.Any("option", selectedBBOption))
		}
	}
})
