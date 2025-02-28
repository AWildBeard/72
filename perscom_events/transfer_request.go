package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const transferRequestCustomID = "transfer-request"
const transferRequestModalCustomID = "transfer-request-modal"
const transferRequestModalSubmitCustomID = "transfer-request-modal-submit"

//go:embed transfer_request_description.txt
var transferRequestDescription string

var transferRequest = ButtonEventHandler{
	Button:         discord.NewPrimaryButton("Transfer", transferRequestCustomID),
	EventListeners: []bot.EventListener{transferRequestEventListener, transferRequestModalEventListener, transferRequestModalSubmitEventListener},
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
			AddActionRow(discord.NewPrimaryButton("Add Current & Desired", transferRequestModalCustomID)).
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
			AddActionRow(discord.NewShortTextInput("from", "Current")).
			AddActionRow(discord.NewShortTextInput("to", "Desired")).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating modal", slog.Any("err", err))
		}
	}
})

var transferRequestModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == transferRequestModalSubmitCustomID {
		// TODO: Create channel and pass data

		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("Submitted your transfer request.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
