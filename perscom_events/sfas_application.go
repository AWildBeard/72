package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const sfasApplicationCustomID = "sfas-application"
const sfasApplicationURL = "https://docs.google.com/forms/d/e/1FAIpQLSda2f6RpfVpgy7PXk3bKRmhCc6EIRpEMotDw4Mi9rRgISmgYg/viewform"

//go:embed sfas_application_description.txt
var sfasApplicationDescription string

var sfasApplication = ButtonEventHandler{
	discord.NewSuccessButton("Special Forces", sfasApplicationCustomID),
	[]bot.EventListener{sfasApplicationEventListener},
}

var sfasApplicationEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == sfasApplicationCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("SFOD-A 072").
				SetColor(0x237f44).
				SetDescription(sfasApplicationDescription).
				Build()).
			AddActionRow(discord.NewLinkButton("SFAS Application", sfasApplicationURL)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})
