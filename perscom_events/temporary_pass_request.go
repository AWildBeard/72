package perscom_events

import (
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
	"time"
)

const temporaryPassRequestCustomID = "temporary-pass-request"

var temporaryPassRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Temporary Pass Request", temporaryPassRequestCustomID),
	[]bot.EventListener{temporaryPassRequestEventListener},
}

var temporaryPassRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == temporaryPassRequestCustomID {
		//TODO: Submit the TPR in the S1 Forum

		// Ensure today's time is set to noon (12:00 PM) UTC
		today := time.Now().UTC()
		today = time.Date(today.Year(), today.Month(), today.Day(), 23, 0, 0, 0, time.UTC)

		offset := (6 - int(today.Weekday()) + 7) % 7 // Calculate days until next Saturday
		nextSaturday := today.AddDate(0, 0, offset)  // Add the offset to today's date to get next Saturday

		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Temporary pass request submitted for the operation <t:" + fmt.Sprintf("%d", nextSaturday.Unix()) + ":R>.").
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})
