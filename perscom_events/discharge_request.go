package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const dischargeRequestCustomID = "discharge-request"
const dischargeRequestWithoutStatementCustomID = "discharge-request-without-statement"
const dischargeRequestStatementModalCustomID = "discharge-request-statement-modal"
const dischargeRequestStatementSubmitModalCustomID = "discharge-request-statement-modal-submit"

//go:embed discharge_request_description.txt
var dischargeRequestDescription string

var dischargeRequest = ButtonEventHandler{
	discord.NewDangerButton("Discharge", dischargeRequestCustomID),
	[]bot.EventListener{dischargeRequestEventHandler,
		dischargeRequestWithoutStatementEventListener,
		dischargeRequestStatementModalEventListener,
		dischargeRequestStatementModalSubmissionEventListener,
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
				discord.NewPrimaryButton("Leave with a Statement", dischargeRequestStatementModalCustomID),
				discord.NewSecondaryButton("Leave without a Statement", dischargeRequestWithoutStatementCustomID),
			).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var dischargeRequestWithoutStatementEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == dischargeRequestWithoutStatementCustomID {
		// TODO: Submit discharge request without statement
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("Submitted your discharge request.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
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

var dischargeRequestStatementModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == dischargeRequestStatementSubmitModalCustomID {
		// TODO: Submit discharge request with statement
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("Submitted your discharge request.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
