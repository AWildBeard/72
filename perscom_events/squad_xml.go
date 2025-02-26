package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const squadXMLCustomID = "squad-xml-button"
const squadXMLCreateModalCustomID = "squad-xml-modal"
const squadXMLSubmitModalCustomID = "squad-xml-modal-submit"

var squadXML = ButtonEventHandler{
	discord.NewPrimaryButton("Request a Squad XML", squadXMLCustomID),
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
					SetDescription("Depending on your section, you'll need one of the following links\n" +
						"1. Platoon: `https://72ndairborne.com/squadxml/airborne/squad.xml`\n" +
						"2. ACE: `https://72ndairborne.com/squadxml/aviation/squad.xml`\n" +
						"3. ODA: `https://72ndairborne.com/squadxml/oda/squad.xml`\n" +
						"Paste the appropriate link for you into the Squad URL section of your ArmA3 Profile.\n\n" +
						"Retrieve:\n" +
						"1. Your Name. This should be your platform name (I.E. `SSG A. Hydra`)\n" +
						"2. Your Player ID. You can get your Player ID from the ArmA3 Profile Menu.\n\n" +
						"When your ready to continue, use the button below to add your Name and " +
						"Player ID to the request.\n\n" +
						"Be sure to put in a Squad XML request with every promotion so we can ensure your unit " +
						"patch stays with you in-game.",
					).
					SetColor(0x00FF00).
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
