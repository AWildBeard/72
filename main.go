package main

import (
	"72/perscom_events"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
)

var (
	token        string
	buildType    string
	buildVersion string
	client       bot.Client
)

func main() {
	slog.Info("starting bot...")
	slog.Info("build version", slog.String("version", buildType+"-"+buildVersion))
	slog.Info("disgo version", slog.String("version", disgo.Version))

	// Read the token from environment variable
	token = os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		slog.Error("DISCORD_BOT_TOKEN is not set! Please check your environment or .env file.")
		return
	} else {
		slog.Info("DISCORD_BOT_TOKEN was loaded", slog.Int("length", len(token)))
	}

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
		warningButtons := make([]discord.ButtonComponent, 0)
		successButtons := make([]discord.ButtonComponent, 0)
		for _, buttonEventHandler := range perscom_events.GetButtonEventHandlers() {
			switch buttonEventHandler.Button.Style {
			case discord.ButtonStylePremium: // We don't use because we don't sell things
			case discord.ButtonStyleSuccess: // Green
				successButtons = append(successButtons, buttonEventHandler.Button)
			case discord.ButtonStylePrimary: // Blue
				primaryButtons = append(primaryButtons, buttonEventHandler.Button)
			case discord.ButtonStyleSecondary: // Gray
				fallthrough
			case discord.ButtonStyleLink: // Also gray?
				fallthrough
			case discord.ButtonStyleDanger: // Red
				warningButtons = append(warningButtons, buttonEventHandler.Button)
			default:
				slog.Error("unknown button style", slog.Any("style", buttonEventHandler.Button.Style))
				return
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
					messages, err := client.Rest().GetMessages(channel.ID(), 0, 0, 0, 100)
					if err != nil {
						slog.Error("error while fetching messages", slog.Any("err", err))
						return
					}

					found := false
					for _, message := range messages {
						if message.Author.ID == client.ID() {
							found = true
							break
						}
					}

					if !found {
						sendButtonsBy5(client, primaryButtons, channel.ID())
						sendButtonsBy5(client, successButtons, channel.ID())
						sendButtonsBy5(client, warningButtons, channel.ID())
					}
				}
			}
		}))
	}

	if err = client.OpenGateway(context.TODO()); err != nil {
		slog.Error("error while connecting to gateway", slog.Any("err", err))
		return
	}

	slog.Info("bot is now running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s
}

func sendButtonsBy5(client bot.Client, buttons []discord.ButtonComponent, channelID snowflake.ID) {
	for i := 0; i < len(buttons); i += 5 {
		end := i + 5
		if end > len(buttons) {
			end = len(buttons)
		}
		buttonGroup := make([]discord.InteractiveComponent, 0)
		for _, button := range buttons[i:end] {
			buttonGroup = append(buttonGroup, button)
		}

		_, err := client.Rest().CreateMessage(channelID, discord.NewMessageCreateBuilder().
			AddActionRow(buttonGroup...).
			Build())

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
}
