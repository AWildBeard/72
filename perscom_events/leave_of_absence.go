package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const leaveOfAbsenceCustomID = "leave-of-absence"
const leaveOfAbsenceModalCustomID = "leave-of-absence-modal"

var leaveOfAbsence = ButtonEventHandler{
	discord.NewPrimaryButton("Leave of absence", leaveOfAbsenceCustomID),
	[]bot.EventListener{leaveOfAbsenceEventListener, leaveOfAbsenceModalSubmissionEventListener},
}

var leaveOfAbsenceEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Leave of Absence").
				SetCustomID(leaveOfAbsenceModalCustomID).
				AddActionRow(discord.NewShortTextInput("reason", "Reason")).
				AddActionRow(discord.NewShortTextInput("date", "Return Date")).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var leaveOfAbsenceModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == leaveOfAbsenceModalCustomID {
		//TODO: Submit LoA Request

		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Leave of absence request submitted.").
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})
