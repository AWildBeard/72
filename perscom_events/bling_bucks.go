package perscom_events

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const blingBucksCustomID = "bling-bucks"
const selectedBBOptionCustomID = "selected-bb-option"
const blingBucksModalSubmit = "bling-bucks-modal-submit"

//const BBForumThreadID = snowflake.ID(1383955310200488107)
//const BBApprovalChannelID = snowflake.ID(1382136230069928046)

const BBForumThreadID = snowflake.ID(1385734566098112582)    // for 72nd server
const BBApprovalChannelID = snowflake.ID(645668825668517888) // for 72nd server

const BBApprovePrefix = "BB-approve"
const BBDenyPrefix = "BB-deny"
const BBDenyModalPrefix = "BB-deny-modal:"

//go:embed bling_bucks_description.txt
var blingBucksDescription string

type BlingBucks struct {
	UserID      string
	Name        string
	PlayerID    string
	Description string
	Nickname    string
	Username    string
	Tickets     string
	Submitted   time.Time
	Approved    *bool
	ReviewedBy  string
	DenyReason  string
	BBOption    string
	ID          string
}

var (
	BBRequests        []BlingBucks
	BBRequestsMutex   sync.Mutex
	BBForumMessageID  snowflake.ID
	BBForumAdded      = make(map[string]bool)
	BBForumAddedMutex sync.Mutex
)

var blingBucksRequest = ButtonEventHandler{
	Button: discord.NewPrimaryButton("Bling Bucks", blingBucksCustomID),
	EventListeners: []bot.EventListener{
		blingBucksEventListener,
		blingBucksSelectedOptionEventListener,
		blingBucksModalSubmitEventListener,
		BBModalSubmissionEventListener,
		BBApprovalButtonListener,
		BBDenyModalListener,
		BBListCommandListener,
		bot.NewListenerFunc(BBClearCommandListener),
	},
}

var blingBucksEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == blingBucksCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetColor(0xe8b923).
				SetTitle(":coin: Bling Bucks Request :coin:").
				SetDescription(blingBucksDescription).
				Build(),
			).
			AddActionRow(discord.NewStringSelectMenu(selectedBBOptionCustomID, "Select an option...",
				discord.NewStringSelectMenuOption("Raffle Ticket - 2 BB", "Raffle Ticket"),
				discord.NewStringSelectMenuOption("Helmet - 8 BB", "Helmet"),
				discord.NewStringSelectMenuOption("Insignia - 10 BB", "Insignia"),
				discord.NewStringSelectMenuOption("Uniform - 10 BB", "Uniform"),
				discord.NewStringSelectMenuOption("Backpack - 10 BB", "Backpack"),
				discord.NewStringSelectMenuOption("Vest - 12 BB", "Vest"),
				discord.NewStringSelectMenuOption("Face-wear - 16 BB", "Face-wear"),
				discord.NewStringSelectMenuOption("Tattoo - 18 BB", "Tattoo"),
			)).
			Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var blingBucksSelectedOptionEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == selectedBBOptionCustomID {
		var err error
		selectedOption := event.StringSelectMenuInteractionData().Values[0]

		if selectedOption == "Raffle Ticket" {
			err = event.Modal(discord.NewModalCreateBuilder().
				SetTitle("Bling Bucks Request").
				SetCustomID(blingBucksModalSubmit + ":" + selectedOption).
				AddActionRow(discord.NewShortTextInput("numTicket", "Number of Tickets (max 3)")).
				Build(),
			)
		} else {
			err = event.Modal(discord.NewModalCreateBuilder().
				SetTitle("Bling Bucks Request").
				SetCustomID(blingBucksModalSubmit + ":" + selectedOption).
				// AddActionRow(discord.NewShortTextInput("name", "In-Game Name (must be exact)")).
				// AddActionRow(discord.NewShortTextInput("player_id", "Player ID")).
				AddActionRow(discord.NewParagraphTextInput("description", "Description (class name and link)")). // this has a character limit for the label!!!
				Build(),
			)
		}

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var blingBucksModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.Contains(event.ModalSubmitInteraction.Data.CustomID, blingBucksModalSubmit) {
		var err error
		selectedBBOption := ""
		if split := strings.Split(event.ModalSubmitInteraction.Data.CustomID, ":"); len(split) == 2 {
			selectedBBOption = split[1]
		} else {
			slog.Error("error while splitting custom ID")
		}

		numTicket, _ := event.Data.TextInputComponent("numTicket")
		if selectedBBOption == "Raffle Ticket" {
			if numTicket.Value != "1" && numTicket.Value != "2" && numTicket.Value != "3" {
				_ = event.CreateMessage(discord.NewMessageCreateBuilder().
					SetEphemeral(true).
					SetContent("Please enter a number between 1 and 3 for Number of Tickets.").
					Build())
				return
			}
		} else {
			err = event.UpdateMessage(discord.NewMessageUpdateBuilder().
				ClearEmbeds().
				ClearContainerComponents().
				SetContent("✅ Your BB request has been submitted. You will receive updates via DM.").
				Build(),
			)

			if err != nil {
				slog.Error("error while updating message", slog.Any("err", err), slog.Any("option", selectedBBOption))
			}
		}
	}
})

var BBModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, blingBucksModalSubmit+":") {
		PlayerID, _ := event.Data.TextInputComponent("player_id")
		Name, _ := event.Data.TextInputComponent("name")
		Description, _ := event.Data.TextInputComponent("description")
		numTicket, _ := event.Data.TextInputComponent("numTicket")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		selectedBBOption := ""
		if split := strings.Split(event.ModalSubmitInteraction.Data.CustomID, ":"); len(split) == 2 {
			selectedBBOption = split[1]
		} else {
			slog.Error("error while splitting custom ID")
		}

		BB := BlingBucks{
			UserID:      event.User().ID.String(),
			Name:        Name.Value,
			PlayerID:    PlayerID.Value,
			Description: Description.Value,
			Username:    event.User().Tag(),
			Nickname:    nickname,
			Submitted:   time.Now().UTC(),
			Tickets:     numTicket.Value,
			BBOption:    selectedBBOption,
			ID:          snowflake.New(time.Now()).String(),
		}

		BBRequestsMutex.Lock()
		BBRequests = append(BBRequests, BB)
		BBRequestsMutex.Unlock()

		if selectedBBOption == "Raffle Ticket" {
			if numTicket.Value != "1" && numTicket.Value != "2" && numTicket.Value != "3" {
				_ = event.CreateMessage(discord.NewMessageCreateBuilder().
					SetEphemeral(true).
					SetContent("Please enter a number between 1 and 3 for Number of Tickets.").
					Build())
				return
			}

			_, err := event.Client().Rest().CreateMessage(
				BBApprovalChannelID,
				discord.NewMessageCreateBuilder().
					SetContent(fmt.Sprintf("BB **%s** request submitted by **%s**\n• Number of Tickets: **%s**",
						selectedBBOption, BB.Nickname, BB.Tickets)).
					AddActionRow(
						discord.NewPrimaryButton("Approve", BBApprovePrefix+BB.ID),
						discord.NewDangerButton("Deny", BBDenyPrefix+BB.ID),
					).
					Build(),
			)
			if err != nil {
				slog.Error("error while creating approval message", slog.Any("err", err))
			}
		} else {
			_, err := event.Client().Rest().CreateMessage(
				BBApprovalChannelID,
				discord.NewMessageCreateBuilder().
					SetContent(fmt.Sprintf("BB **%s** request submitted by **%s**\n• Description: **%s**",
						selectedBBOption, BB.Nickname, BB.Description)).
					AddActionRow(
						discord.NewPrimaryButton("Approve", BBApprovePrefix+BB.ID),
						discord.NewDangerButton("Deny", BBDenyPrefix+BB.ID),
					).
					Build(),
			)
			if err != nil {
				slog.Error("error while creating approval message", slog.Any("err", err))
			}
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(event.User().ID)
		if err != nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your BB request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your BB request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var BBApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, BBApprovePrefix) {
		reqID := strings.TrimPrefix(customID, BBApprovePrefix)
		BBRequestsMutex.Lock()
		for i := range BBRequests {
			if BBRequests[i].ID == reqID {
				approved := true
				BBRequests[i].Approved = &approved
				BBRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		BBRequestsMutex.Unlock()

		updateBBForumPost(event.Client())

		// Find the request by ID and get the UserID
		var userID string
		BBRequestsMutex.Lock()
		for i := range BBRequests {
			if BBRequests[i].ID == reqID {
				userID = BBRequests[i].UserID
				// ... (update fields as before)
				break
			}
		}
		BBRequestsMutex.Unlock()

		if userID != "" {
			dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
			if err == nil {
				_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
					discord.NewMessageCreateBuilder().
						SetContent("✅ Your BB request has been **approved**.").
						Build(),
				)
			}
		}

		_ = event.Client().Rest().DeleteMessage(BBApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, BBDenyPrefix) {
		userID := strings.TrimPrefix(customID, BBDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny BB Request").
				SetCustomID("BB-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var BBDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, BBDenyModalPrefix) {
		reqID := strings.TrimPrefix(event.Data.CustomID, BBDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		BBRequestsMutex.Lock()
		for i := range BBRequests {
			if BBRequests[i].ID == reqID {
				approved := false
				BBRequests[i].Approved = &approved
				BBRequests[i].ReviewedBy = event.User().Tag()
				BBRequests[i].DenyReason = reason.Value
				break
			}
		}
		BBRequestsMutex.Unlock()

		updateBBForumPost(event.Client())

		// Find the request by ID and get the UserID
		var userID string
		BBRequestsMutex.Lock()
		for i := range BBRequests {
			if BBRequests[i].ID == reqID {
				userID = BBRequests[i].UserID
				// ... (update fields as before)
				break
			}
		}
		BBRequestsMutex.Unlock()

		if userID != "" {
			dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
			if err == nil {
				_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
					discord.NewMessageCreateBuilder().
						SetContentf("❌ Your BB request has been **denied**.\n**Reason:** %s", reason.Value).
						Build(),
				)
			}
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("BB request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(BBApprovalChannelID, event.Message.ID)
	}
})

var BBListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() == "bb-list" {
		BBRequestsMutex.Lock()
		defer BBRequestsMutex.Unlock()
		if len(BBRequests) == 0 {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent("No current requests.").
				Build())
			return
		}

		var sb strings.Builder
		//var allNicknames []string
		sb.WriteString("**Current BB Requests:**\n")
		for _, req := range BBRequests {
			//allNicknames = append(allNicknames, req.NickName)
			status := "🕐 pending"
			if req.Approved != nil {
				if *req.Approved {
					status = "✅ approved by " + req.ReviewedBy
				} else {
					event.CreateMessage(discord.NewMessageCreateBuilder().
						SetContent("No current requests.").
						Build())
					return
				}
				sb.WriteString(fmt.Sprintf(
					"\n• BB **%s** submitted by **%s** at %s with Description: **%s** (Status: %s)\n",
					req.BBOption,
					req.Nickname,
					req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
					req.Description,
					status,
				))
			}
		}

		//slog.Info("Nicknames on file", slog.Any("nicknames", allNicknames))

		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(sb.String()).
			Build())
	}
})

func BBClearCommandListener(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() != "bb-clear" {
		return
	}

	//slog.Info("BB-CLEAR command triggered")

	userNameToClear := ""

	// Extract the nickname option if it's present
	if data, ok := event.Data.(discord.SlashCommandInteractionData); ok {
		//slog.Info("BB-CLEAR options", slog.Any("options", data.Options))
		for _, opt := range data.Options {
			//slog.Info("BB-CLEAR option", slog.String("name", opt.Name), slog.String("type", fmt.Sprintf("%v", opt.Type)), slog.Any("value", opt.Value))
			if opt.Name == "nickname" && opt.Type == discord.ApplicationCommandOptionTypeString {
				var extracted string
				if err := json.Unmarshal(opt.Value, &extracted); err != nil {
					slog.Error("Failed to unmarshal option value", slog.Any("error", err))
				} else {
					userNameToClear = strings.TrimSpace(extracted)
				}
				break
			}
		}
	}

	//slog.Info("Proceeding to BB clear logic", slog.String("userNameToClear", userNameToClear))

	BBRequestsMutex.Lock()
	defer BBRequestsMutex.Unlock()

	if userNameToClear != "" {
		found := false
		userInputLower := strings.ToLower(userNameToClear)

		const minInputLen = 3
		if len(userInputLower) < minInputLen {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Please provide at least %d characters to clear a request.", minInputLen)).
				Build())
			return
		}

		for i, BB := range BBRequests {
			nickLower := strings.ToLower(BB.Nickname)

			// Check exact match on full nickname
			if nickLower == userInputLower {
				BBRequests = append(BBRequests[:i], BBRequests[i+1:]...)
				found = true
				break
			}

			// Otherwise, fuzzy match on last word
			parts := strings.Fields(BB.Nickname)
			if len(parts) > 0 {
				lastWord := strings.ToLower(parts[len(parts)-1])
				if strings.Contains(lastWord, userInputLower) {
					BBRequests = append(BBRequests[:i], BBRequests[i+1:]...)
					found = true
					break
				}
			}
		}

		if found {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Cleared request for %s.", userNameToClear)).
				Build())
		} else {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("No request found for %s.", userNameToClear)).
				Build())
		}
	} else {
		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("Please specify a nickname to clear.").
			Build())
	}
}

func updateBBForumPost(client bot.Client) {
	BBRequestsMutex.Lock()
	defer BBRequestsMutex.Unlock()

	BBForumAddedMutex.Lock()
	BBForumAdded = make(map[string]bool)
	BBForumAddedMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range BBRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%s|%s|%v|%s", req.UserID, req.BBOption, req.Tickets, req.PlayerID, req.Approved, req.Submitted.Format(time.RFC3339Nano))

		BBForumAddedMutex.Lock()
		if BBForumAdded[key] {
			BBForumAddedMutex.Unlock()
			continue
		}
		BBForumAdded[key] = true
		BBForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = "✅ approved by " + req.ReviewedBy
		} else {
			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
		}

		// Detect Raffle Ticket by checking if Tickets is set
		if req.Tickets != "" {
			newEntries.WriteString(fmt.Sprintf(
				"\n• Raffle Ticket submitted by **%s** at %s — Number of Tickets: **%s** (Status: %s)\n",
				req.Nickname,
				req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
				req.Tickets,
				status,
			))
		} else {
			newEntries.WriteString(fmt.Sprintf(
				"\n• BB **%s** submitted by **%s** at %s Description: **%s** (Status: %s)\n",
				req.BBOption,
				req.Nickname,
				req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
				req.Description,
				status,
			))
		}
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if BBForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(BBForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**BB Log:**\n"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create BB forum post", slog.Any("err", err))
			return
		}
		BBForumMessageID = msg.ID
	} else {
		// Overwrite the existing message with the full log
		newContent := "**BB Log:**\n" + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}
		_, err := client.Rest().UpdateMessage(BBForumThreadID, BBForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update BB forum post", slog.Any("err", err))
		}
	}
}
