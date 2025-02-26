package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const leaveOfAbsenceCustomID = "leave-of-absence"
const leaveOfAbsenceModalCustomID = "leave-of-absence-modal"
const leaveOfAbsenceModalSubmissionCustomID = "leave-of-absence-modal-submit"

var leaveOfAbsence = ButtonEventHandler{
	discord.NewPrimaryButton("Leave of absence", leaveOfAbsenceCustomID),
	[]bot.EventListener{leaveOfAbsenceEventListener, leaveOfAbsenceModalEventListener, leaveOfAbsenceModalSubmissionEventListener},
}

var leaveOfAbsenceEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Leave of Absence").
				SetColor(0x00FF00).
				SetDescription(
					`A Leave of Absence (LOA) is an extended block of time, ranging from 2-weeks to 2-months, where you'll be inactive within the community. An LOA is typically used for vacations, work trips, military functions, or any reason that requires you to be away from community for at least 2-weeks.

By submitting an LOA request you're notifying us of your intended absence so we do not mistake you for a person "Away Without Leave." When you return from your LOA, check in with your direct superior immediately. One of our administrative staff will then transfer you back to your respective combat unit.

**TYPES OF LOA**
- Standard LOA is what has been previously described. This is what most people use.
- Military Leave of Absence (MLOA). This may be requested for community members actively serving in the armed forces, this extends your LOA from 6 to 12-months. Be sure to specify an MLOA in your request if this applies.
- Emergency Leave of Absence (ELOA). This may be requested for community members needing an extended period of time away from the community due to emergency such as private medical reasons. Be sure to specify an ELOA in your request if this applies.

**EXTENDING YOUR LOA**
You can extend your LOA at the end of your 2-month period by completing the same process: submit a Leave of Absence (LOA) Request.

**LOA RESTRICTIONS**
1. Recruits are ineligible to submit an LOA Request.
2. LOA Requests under 2-weeks will be denied. Use a TPR.
3. Requests over 2-months will be denied (MLOA and ELOA exluded). Consider discharging to reserves or retirement if you need longer.`).
				Build(),
			).
			AddActionRow(discord.NewPrimaryButton("Submit Leave of Absence Request", leaveOfAbsenceModalCustomID)).
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
