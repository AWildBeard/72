package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const leaveOfAbsenceCustomID = "leave-of-absence"
const leaveOfAbsenceModalCustomID = "leave-of-absence-modal"
const leaveOfAbsenceModalSubmissionCustomID = "leave-of-absence-modal-submit"

//go:embed leave_of_absence_description.txt
var leaveOfAbsenceDescription string

var leaveOfAbsence = ButtonEventHandler{
	discord.NewPrimaryButton("Leave of Absence", leaveOfAbsenceCustomID),
	[]bot.EventListener{leaveOfAbsenceEventListener, leaveOfAbsenceModalEventListener, leaveOfAbsenceModalSubmissionEventListener},
}

var leaveOfAbsenceEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Leave of Absence").
				SetColor(0x5765f2).
				SetDescription(leaveOfAbsenceDescription).
				Build(),
			).
			AddActionRow(discord.NewPrimaryButton("Add Details & Submit", leaveOfAbsenceModalCustomID)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var leaveOfAbsenceModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Leave of Absence").
				SetCustomID(leaveOfAbsenceModalSubmissionCustomID).
				AddActionRow(discord.NewShortTextInput("reason", "Reason")).
				AddActionRow(discord.NewShortTextInput("date", "Approx Return Date")).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var leaveOfAbsenceModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == leaveOfAbsenceModalSubmissionCustomID {
		//TODO: Submit LoA Request

		err := event.UpdateMessage(
			discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent("Leave of absence request submitted.").
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})
