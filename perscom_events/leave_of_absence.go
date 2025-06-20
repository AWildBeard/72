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

const leaveOfAbsenceCustomID = "leave-of-absence"
const leaveOfAbsenceModalCustomID = "leave-of-absence-modal"
const leaveOfAbsenceModalSubmissionCustomID = "leave-of-absence-modal-submit"

const loaForumThreadID = snowflake.ID(1382882958314307604)
const loaApprovalChannelID = snowflake.ID(1382136230069928046)

const loaApprovePrefix = "loa-approve"
const loaDenyPrefix = "loa-deny"
const loaDenyModalPrefix = "loa-deny-modal:"

//go:embed leave_of_absence_description.txt
var leaveOfAbsenceDescription string

type LeaveOfAbsence struct {
	UserID     string
	UserName   string
	NickName   string
	Reason     string
	ReturnETA  string
	Submitted  time.Time
	Approved   *bool
	ReviewedBy string
	DenyReason string
}

var (
	leaveRequests      []LeaveOfAbsence
	leaveRequestsMutex sync.Mutex
	loaForumMessageID  snowflake.ID
	loaForumAdded      = make(map[string]bool)
	loaForumAddedMutex sync.Mutex
)

var leaveOfAbsence = ButtonEventHandler{
	discord.NewPrimaryButton("Leave of Absence", leaveOfAbsenceCustomID),
	[]bot.EventListener{
		leaveOfAbsenceEventListener,
		leaveOfAbsenceModalEventListener,
		leaveOfAbsenceModalSubmissionEventListener,
		loaListCommandListener,
		bot.NewListenerFunc(loaClearCommandListener),
		loaApprovalButtonListener,
		loaDenyModalListener,
	},
}

var leaveOfAbsenceEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceCustomID {
		err := event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetEmbeds(discord.NewEmbedBuilder().
				SetTitle("Leave of Absence").
				SetColor(0x5765f2).
				SetDescription(leaveOfAbsenceDescription).
				Build()).
			AddActionRow(discord.NewPrimaryButton("Add Details & Submit", leaveOfAbsenceModalCustomID)).
			Build())

		if err != nil {
			slog.Error("error while creating LOA message", slog.Any("err", err))
		}
	}
})

var leaveOfAbsenceModalEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == leaveOfAbsenceModalCustomID {
		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Leave of Absence").
				SetCustomID(leaveOfAbsenceModalSubmissionCustomID).
				AddActionRow(discord.NewShortTextInput("reason", "Reason")).
				AddActionRow(discord.NewShortTextInput("date", "Approx Return Date")).
				Build(),
		)

		if err != nil {
			slog.Error("error while showing LOA modal", slog.Any("err", err))
		}
	}
})

var leaveOfAbsenceModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if event.Data.CustomID == leaveOfAbsenceModalSubmissionCustomID {
		reason, _ := event.Data.TextInputComponent("reason")
		date, _ := event.Data.TextInputComponent("date")

		// Get the user's nickname or fallback to username
		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		loa := LeaveOfAbsence{
			UserID:    event.User().ID.String(),
			UserName:  event.User().Tag(),
			NickName:  nickname,
			Reason:    reason.Value,
			ReturnETA: date.Value,
			Submitted: time.Now().UTC(),
		}

		leaveRequestsMutex.Lock()
		leaveRequests = append(leaveRequests, loa)
		leaveRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			loaApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("📝 LOA request submitted by <@%s>\n• Reason: %s\n• Return Date: %s",
					loa.UserID, loa.Reason, loa.ReturnETA)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", loaApprovePrefix+loa.UserID),
					discord.NewDangerButton("Deny", loaDenyPrefix+loa.UserID),
				).
				Build(),
		)
		if err != nil {
			slog.Error("failed to post LOA approval message", slog.Any("err", err))
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(event.User().ID)
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your leave of absence request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your leave of absence request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var loaApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, loaApprovePrefix) {
		userID := strings.TrimPrefix(customID, loaApprovePrefix)

		leaveRequestsMutex.Lock()
		for i := range leaveRequests {
			if leaveRequests[i].UserID == userID {
				approved := true
				leaveRequests[i].Approved = &approved
				leaveRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		leaveRequestsMutex.Unlock()

		updateLOAForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your LOA request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(loaApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, loaDenyPrefix) {
		userID := strings.TrimPrefix(customID, loaDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny LOA Request").
				SetCustomID("loa-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var loaDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, loaDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, loaDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		leaveRequestsMutex.Lock()
		for i := range leaveRequests {
			if leaveRequests[i].UserID == userID {
				approved := false
				leaveRequests[i].Approved = &approved
				leaveRequests[i].ReviewedBy = event.User().Tag()
				leaveRequests[i].DenyReason = reason.Value
				break
			}
		}
		leaveRequestsMutex.Unlock()

		updateLOAForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your LOA request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("LOA denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(loaApprovalChannelID, event.Message.ID)
	}
})

var loaListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() == "loa-list" {
		leaveRequestsMutex.Lock()
		defer leaveRequestsMutex.Unlock()
		if len(leaveRequests) == 0 {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent("No current LOAs.").
				Build())
			return
		}

		var sb strings.Builder
		//var allNicknames []string
		sb.WriteString("**Current Leaves of Absence:**\n")
		for _, req := range leaveRequests {
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
					"\n• <@%s> until (approximate return date): **%s** — reason: **%s** (%s)\n",
					req.UserID, req.ReturnETA, req.Reason, status,
				))
			}
		}

		//slog.Info("Nicknames on file", slog.Any("nicknames", allNicknames))

		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent(sb.String()).
			Build())
	}
})

func loaClearCommandListener(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() != "loa-clear" {
		return
	}

	//slog.Info("LOA-CLEAR command triggered")

	userNameToClear := ""

	// Extract the nickname option if it's present
	if data, ok := event.Data.(discord.SlashCommandInteractionData); ok {
		//slog.Info("LOA-CLEAR options", slog.Any("options", data.Options))
		for _, opt := range data.Options {
			//slog.Info("LOA-CLEAR option", slog.String("name", opt.Name), slog.String("type", fmt.Sprintf("%v", opt.Type)), slog.Any("value", opt.Value))
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

	//slog.Info("Proceeding to LOA clear logic", slog.String("userNameToClear", userNameToClear))

	leaveRequestsMutex.Lock()
	defer leaveRequestsMutex.Unlock()

	if userNameToClear != "" {
		found := false
		userInputLower := strings.ToLower(userNameToClear)

		const minInputLen = 3
		if len(userInputLower) < minInputLen {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Please provide at least %d characters to clear an LOA.", minInputLen)).
				Build())
			return
		}

		for i, loa := range leaveRequests {
			nickLower := strings.ToLower(loa.NickName)

			// Check exact match on full nickname
			if nickLower == userInputLower {
				leaveRequests = append(leaveRequests[:i], leaveRequests[i+1:]...)
				found = true
				break
			}

			// Otherwise, fuzzy match on last word
			parts := strings.Fields(loa.NickName)
			if len(parts) > 0 {
				lastWord := strings.ToLower(parts[len(parts)-1])
				if strings.Contains(lastWord, userInputLower) {
					leaveRequests = append(leaveRequests[:i], leaveRequests[i+1:]...)
					found = true
					break
				}
			}
		}

		if found {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("Cleared LOA for %s.", userNameToClear)).
				Build())
		} else {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("No LOA found for %s.", userNameToClear)).
				Build())
		}
	} else {
		event.CreateMessage(discord.NewMessageCreateBuilder().
			SetContent("Please specify a nickname to clear.").
			Build())
	}
}

func updateLOAForumPost(client bot.Client) {
	leaveRequestsMutex.Lock()
	defer leaveRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range leaveRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%s|%v", req.UserID, req.ReturnETA, req.Reason, req.Approved)

		loaForumAddedMutex.Lock()
		if loaForumAdded[key] {
			loaForumAddedMutex.Unlock()
			continue
		}
		loaForumAdded[key] = true
		loaForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = "✅ approved by " + req.ReviewedBy
		} else {
			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n• %s submitted LOA request at %s until: **%s** reason: **%s** (Status: %s)\n",
			req.NickName,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.ReturnETA,
			req.Reason,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if loaForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(loaForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Leave of Absence Log:**\n"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create LOA forum post", slog.Any("err", err))
			return
		}
		loaForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(loaForumThreadID, loaForumMessageID)
		if err != nil {
			slog.Error("failed to fetch LOA forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(loaForumThreadID, loaForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update LOA forum post", slog.Any("err", err))
		}
	}
}
