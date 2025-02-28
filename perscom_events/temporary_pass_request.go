package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
	"time"
)

const temporaryPassRequestCustomID = "temporary-pass-request"
const temporaryPassRequestSubmitCustomID = "temporary-pass-request-submit"

//go:embed temporary_pass_request_description.txt
var temporaryPassRequestDescription string

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass", temporaryPassRequestCustomID),
	[]bot.EventListener{temporaryPassRequestEventListener, temporaryPassRequestSubmitEventListener},
}

var temporaryPassRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("Temporary Pass Request").
					SetColor(0x5765f2).
					SetDescription(temporaryPassRequestDescription).
					Build(),
				).
				AddActionRow(discord.NewPrimaryButton("Submit", temporaryPassRequestSubmitCustomID)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var temporaryPassRequestSubmitEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestSubmitCustomID {
		//TODO: Submit TPR

		// Ensure today's time is set to noon (12:00 PM) UTC
		today := time.Now().UTC()
		today = time.Date(today.Year(), today.Month(), today.Day(), 23, 0, 0, 0, time.UTC)

		offset := (6 - int(today.Weekday()) + 7) % 7 // Calculate days until next Saturday
		nextSaturday := today.AddDate(0, 0, offset)  // Add the offset to today's date to get next Saturday

		err := event.UpdateMessage(
			discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContentf("Submitted your temporary pass request for the operation <t:%d:R>.", nextSaturday.Unix()).
				Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
