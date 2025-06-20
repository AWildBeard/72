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
	"github.com/joho/godotenv"
)

var (
	token        string
	buildType    string
	buildVersion string
	client       bot.Client
)

func init() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("No .env file found, falling back to system environment variables")
	}
}

func main() {

	// 1. Open the log file
	file, err1 := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err1 != nil {
		slog.Error("failed to open log file", slog.Any("error", err1))
		return
	}
	defer file.Close() // Ensure the file is closed
	// 2. Choose a handler (e.g., JSON handler)
	handler := slog.NewTextHandler(file, nil) // You can add options here if needed
	// 3. Create a new logger
	logger := slog.New(handler)
	// Set the default logger (optional, but good practice for consistent logging)
	slog.SetDefault(logger)

	slog.Info("starting bot...")
	slog.Info("build version", slog.String("version", buildType+"-"+buildVersion))
	slog.Info("disgo version", slog.String("version", disgo.Version))

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

	perscom_events.InitTPRScheduler(client)

	defer client.Close(context.TODO())

	primaryButtons := make([]discord.ButtonComponent, 0)
	warningButtons := make([]discord.ButtonComponent, 0)
	successButtons := make([]discord.ButtonComponent, 0)

	for _, buttonEventHandler := range perscom_events.GetButtonEventHandlers() {
		switch buttonEventHandler.Button.Style {
		case discord.ButtonStylePremium:
		case discord.ButtonStyleSuccess:
			successButtons = append(successButtons, buttonEventHandler.Button)
		case discord.ButtonStylePrimary:
			primaryButtons = append(primaryButtons, buttonEventHandler.Button)
		case discord.ButtonStyleSecondary, discord.ButtonStyleLink, discord.ButtonStyleDanger:
			warningButtons = append(warningButtons, buttonEventHandler.Button)
		default:
			slog.Error("unknown button style", slog.Any("style", buttonEventHandler.Button.Style))
			return
		}

		client.AddEventListeners(buttonEventHandler.EventListeners...)
	}

	client.AddEventListeners(bot.NewListenerFunc(func(event *events.GuildReady) {
		slog.Info("Bot is ready, registering slash commands...")

		// Register slash commands dynamically on bot ready
		_, err := client.Rest().SetGuildCommands(
			client.ID(),
			event.GuildID,
			[]discord.ApplicationCommandCreate{
				discord.SlashCommandCreate{
					Name:        "tpr-list",
					Description: "List all approved temporary pass requests",
				},
				discord.SlashCommandCreate{
					Name:        "loa-list",
					Description: "List all approved leave of absence requests",
				},
				discord.SlashCommandCreate{
					Name:        "loa-clear",
					Description: "Clear leave of absence requests by nickname",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionString{
							Name:        "nickname",
							Description: "The nickname to clear LOAs for",
							Required:    true,
						},
					},
				},
				discord.SlashCommandCreate{
					Name:        "school-list",
					Description: "List all approved school and course requests",
				},
				discord.SlashCommandCreate{
					Name:        "school-clear",
					Description: "Clear school or course requests by nickname",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionString{
							Name:        "nickname",
							Description: "The nickname to clear School or Course requests for",
							Required:    true,
						},
					},
				},
				discord.SlashCommandCreate{
					Name:        "bb-list",
					Description: "List all approved BB requests",
				},
				discord.SlashCommandCreate{
					Name:        "bb-clear",
					Description: "Clear BB requests by nickname",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionString{
							Name:        "nickname",
							Description: "The nickname to clear BB requests for",
							Required:    true,
						},
					},
				},
			},
		)
		if err != nil {
			slog.Error("failed to register slash commands", slog.Any("err", err))
			return
		}

		slog.Info("Slash commands registered successfully")

		channels, err := client.Rest().GetGuildChannels(event.GuildID)
		if err != nil {
			slog.Error("error while getting channels", slog.Any("err", err))
			return
		}

		for _, channel := range channels {
			if channel.Name() == "perscom-requests" {
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

	// Handle slash command interactions
	// client.AddEventListeners(bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	// 	switch event.Data.CommandName() {
	// 	case "tpr-list":
	// 		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
	// 			SetContent("Approved Temporary Pass Requests:\n- Example 1\n- Example 2").
	// 			Build())
	// 	case "loa-list":
	// 		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
	// 			SetContent("Approved Leave of Absence Requests:\n- LOA 1\n- LOA 2").
	// 			Build())
	// 	case "loa-clear":
	// 		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
	// 			SetContent("Clear LOAs by name.").
	// 			Build())
	// 	case "school-list":
	// 		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
	// 			SetContent("Approved School Requests:\n- School 1\n- Course 2").
	// 			Build())
	// 	default:
	// 		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
	// 			SetContent("Unknown command.").
	// 			Build())
	// 	}
	// }))

	if err = client.OpenGateway(context.TODO()); err != nil {
		slog.Error("error while connecting to gateway", slog.Any("err", err))
		return
	}

	slog.Info("bot is now running. Press CTRL-C to exit.")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s

	slog.Info("shutdown signal received, closing client...")
	client.Close(context.TODO())
	slog.Info("bot shut down successfully.")
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
