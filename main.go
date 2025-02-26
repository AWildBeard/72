package main

import (
	"72/perscom_events"
	"context"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

var (
	token        = os.Getenv("disgo_token")
	buildType    string
	buildVersion string

	client bot.Client
)

func main() {
	slog.Info("starting example...")
	slog.Info("version", slog.String("version", buildType+"-"+buildVersion))
	slog.Info("disgo version", slog.String("version", disgo.Version))

	var err error
	client, err = disgo.New(token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
			),
		),
	)

	if err != nil {
		slog.Error("error while building bot", slog.Any("err", err))
		return
	}
	defer client.Close(context.TODO())

	{
		primaryButtons := make([]discord.ButtonComponent, 0)
		for _, buttonEventHandler := range perscom_events.GetButtonEventHandlers() {
			switch buttonEventHandler.Button.Style {
			case discord.ButtonStylePremium:
			case discord.ButtonStyleSuccess: // Green
			case discord.ButtonStylePrimary: // Blue
				primaryButtons = append(primaryButtons, buttonEventHandler.Button)
			case discord.ButtonStyleSecondary: // Gray
			case discord.ButtonStyleLink:
			case discord.ButtonStyleDanger: // Red
			}

			client.AddEventListeners(buttonEventHandler.EventListeners...)
		}

		client.AddEventListeners(bot.NewListenerFunc(func(event *events.GuildReady) {
			channels, err := client.Rest().GetGuildChannels(event.GuildID)
			if err != nil {
				slog.Error("error while getting channels", slog.Any("err", err))
				return
			}

			for _, channel := range channels {
				if channel.Name() == "perscom" {
					// Detect and delete previous "Hello World" messages from the bot
					messages, err := client.Rest().GetMessages(channel.ID(), 0, 0, 0, 100) // Fetch the last 100 messages
					if err != nil {
						slog.Error("error while fetching messages", slog.Any("err", err))
						return
					}

					found := false
					for _, message := range messages {
						if message.Author.ID == client.ID() {
							found = true
							break
							//err := client.Rest().DeleteMessage(channel.ID(), message.ID)
							//if err != nil {
							//	slog.Error("error while deleting message", slog.Any("err", err))
							//}
						}
					}

					if !found {
						// TODO: Success buttons

						for i := 0; i < len(primaryButtons); i += 5 {
							end := i + 5
							if end > len(primaryButtons) {
								end = len(primaryButtons)
							}
							buttonGroup := make([]discord.InteractiveComponent, 0)
							for _, button := range primaryButtons[i:end] {
								buttonGroup = append(buttonGroup, button)
							}

							// Create a new message with buttons in groups of 5
							_, _ = client.Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
								AddActionRow(buttonGroup...).
								Build())
						}
						//
						//buttons := []discord.InteractiveComponent{
						//	discord.NewSuccessButton("Join The Unit", "join-unit-application"),
						//	discord.NewSuccessButton("BB Redemption", "bb-redemption"),
						//}
						//_, _ = client.Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
						//	AddActionRow(buttons...).
						//	Build())
						//
						//_, _ = client.Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
						//	AddActionRow(discord.NewPrimaryButton("School & Course Request", "school-and-course-request"),
						//		discord.NewPrimaryButton("Request a Squad XML", "squad-xml-request"),
						//		discord.NewPrimaryButton("Award Recommendation", "award-recommendation"),
						//		discord.NewPrimaryButton("Squad Transfer Request", "squad-transfer-request")).
						//	Build())
						//
						//_, _ = client.Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
						//	AddActionRow(discord.NewPrimaryButton("Leave of Absense", "leave-of-absense"),
						//		discord.NewPrimaryButton("Temporary Pass Request", "temporary-pass-request")).
						//	Build())
						//
						//_, _ = client.Rest().CreateMessage(channel.ID(), discord.NewMessageCreateBuilder().
						//	AddActionRow(discord.NewSecondaryButton("Special Forces Application", "special-forces-application"),
						//		discord.NewDangerButton("Discharge Request", "discharge-request")).
						//	Build())
						//
					}

					return
				}
			}
		}))
	}

	if err = client.OpenGateway(context.TODO()); err != nil {
		slog.Error("error while connecting to gateway", slog.Any("err", err))
		return
	}

	slog.Info("example is now running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s
}
