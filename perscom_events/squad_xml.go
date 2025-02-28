package perscom_events

import (
	_ "embed"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const squadXMLCustomID = "squad-xml-button"
const squadXMLCreateModalCustomID = "squad-xml-modal"
const squadXMLSubmitModalCustomID = "squad-xml-modal-submit"

//go:embed squad_xml_description.txt
var squadXMLDescription string

var squadXML = ButtonEventHandler{
	discord.NewPrimaryButton("Squad XML", squadXMLCustomID),
	[]bot.EventListener{squadXMLEventListener, squadXMLModalRequestEventListener, squadXMLModalSubmissionEventListener},
}

var squadXMLEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == squadXMLCustomID {
		//TODO: Submit the squad XML request in the S1 Forum

		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("Squad XML Request Instructions").
					SetDescription(squadXMLDescription).
					SetColor(0x5765f2).
					Build(),
				).
				AddActionRow(discord.NewPrimaryButton("Add Name & Player ID", squadXMLCreateModalCustomID)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var squadXMLModalRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == squadXMLCreateModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Squad XML Request").
				SetCustomID(squadXMLSubmitModalCustomID).
				AddActionRow(discord.NewShortTextInput("name", "Name")).
				AddActionRow(discord.NewShortTextInput("player_id", "Player ID")).
				Build())

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var squadXMLModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.ModalSubmitInteraction.Data.CustomID == squadXMLSubmitModalCustomID {
		//TODO: Make forum post for S1 to fulfil
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearContainerComponents().
			ClearEmbeds().
			SetContent("Submitted your Squad XML request.").
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
