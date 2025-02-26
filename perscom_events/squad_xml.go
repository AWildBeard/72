package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const squadXMLCustomID = "squad-xml-button"

var squadXML = ButtonEventHandler{
	discord.NewPrimaryButton("Request a Squad XML", squadXMLCustomID),
	[]bot.EventListener{squadXMLEventListener},
}

var squadXMLEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == squadXMLCustomID {
		//TODO: Submit the squad XML request in the S1 Forum

		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetContent("Squad XML request submitted.").
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}

})
