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

// to use as template for other files replace SandC prefix with another and replace anything that is school or course with another name

const schoolAndCourseRequestCustomID = "school-and-course-request"
const selectedCourseCustomID = "selected-course"
const selectedCourseAvailabilityModalSubmit = "selected-course-availability-modal-submit"

const SandCForumThreadID = snowflake.ID(1383869877688729713)
const SandCApprovalChannelID = snowflake.ID(1382136230069928046)
const SandCApprovePrefix = "sandc-approve"
const SandCDenyPrefix = "sandc-deny"
const SandCDenyModalPrefix = "sandc-deny-modal:"

//go:embed school_and_course_request_description.txt
var schoolAndCourseDescription string

type SchoolAndCourse struct {
	UserID       string
	CourseName   string
	Availability string
	Nickname     string
	Username     string
	Submitted    time.Time
	Approved     *bool
	ReviewedBy   string
	DenyReason   string
}

var (
	SandCRequests        []SchoolAndCourse
	SandCRequestsMutex   sync.Mutex
	SandCForumMessageID  snowflake.ID
	SandCForumAdded      = make(map[string]bool)
	SandCForumAddedMutex sync.Mutex
)

var schoolAndCourseRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Schools & Courses", schoolAndCourseRequestCustomID),
	[]bot.EventListener{
		schoolAndCourseRequestEventListener,
		schoolAndCourseRequestSelectionEventListener,
		schoolAndCourseModalSubmitEventListener,
		SandCModalSubmissionEventListener,
		SandCListCommandListener,
		bot.NewListenerFunc(SandCClearCommandListener),
		SandCApprovalButtonListener,
		SandCDenyModalListener,
	},
}

var schoolAndCourseRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == schoolAndCourseRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("School & Course Descriptions").
					SetDescription(schoolAndCourseDescription).
					SetColor(0x5765f2). // Example color
					Build()).
				AddActionRow(discord.NewStringSelectMenu(selectedCourseCustomID, "Select a school or course",
					discord.NewStringSelectMenuOption("Airborne", "Airborne"),
					discord.NewStringSelectMenuOption("Air Assault", "Air Assault"),
					discord.NewStringSelectMenuOption("Advanced Infantry Training", "Advanced Infantry Training"),
					discord.NewStringSelectMenuOption("Ranger School", "Ranger School"),
					discord.NewStringSelectMenuOption("Combat Life Saver", "Combat Life Saver"),
					discord.NewStringSelectMenuOption("Drill Instructor Course", "Drill Instructor Course"),
					discord.NewStringSelectMenuOption("NCO Training & Leadership", "NCO Training & Leadership"),
					discord.NewStringSelectMenuOption("Squad Designated Marksman (SDM)", "Squad Designated Marksman (SDM)"),
					discord.NewStringSelectMenuOption("Explosive Ordnance Disposal (EOD)", "Explosive Ordnance Disposal (EOD)"),
					discord.NewStringSelectMenuOption("Electronic Warfare Specialist (EWAR)", "Electronic Warfare Specialist (EWAR)"),
				)).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var schoolAndCourseRequestSelectionEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == selectedCourseCustomID {
		err := event.Modal(discord.NewModalCreateBuilder().
			SetTitle("Attendee Availability").
			SetCustomID(selectedCourseAvailabilityModalSubmit + fmt.Sprintf(":%v", event.StringSelectMenuInteractionData().Values[0])).
			AddActionRow(discord.NewShortTextInput("availability", "Availability")).
			Build(),
		)

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})

var schoolAndCourseModalSubmitEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.Contains(event.ModalSubmitInteraction.Data.CustomID, selectedCourseAvailabilityModalSubmit) {
		course := ""
		if split := strings.Split(event.ModalSubmitInteraction.Data.CustomID, ":"); len(split) == 2 {
			course = split[1]
		} else {
			slog.Error("error while splitting custom ID")
			return
		}

		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContentf("Submitted your request for \"%v\".", course).
			Build())

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}

	}
})

var SandCModalSubmissionEventListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, selectedCourseAvailabilityModalSubmit+":") {
		parts := strings.SplitN(event.Data.CustomID, ":", 2)
		course := ""
		if len(parts) == 2 {
			course = parts[1]
		}

		availability, _ := event.Data.TextInputComponent("availability")

		var nickname string
		if event.Member() != nil && event.Member().Nick != nil {
			nickname = *event.Member().Nick
		} else {
			nickname = event.User().Username
		}

		SandC := SchoolAndCourse{
			UserID:       event.User().ID.String(),
			CourseName:   course,
			Availability: availability.Value,
			Username:     event.User().Tag(),
			Nickname:     nickname,
			Submitted:    time.Now().UTC(),
		}

		SandCRequestsMutex.Lock()
		SandCRequests = append(SandCRequests, SandC)
		SandCRequestsMutex.Unlock()

		_, err := event.Client().Rest().CreateMessage(
			SandCApprovalChannelID,
			discord.NewMessageCreateBuilder().
				SetContent(fmt.Sprintf("**%s** request submitted by **%s**\n• Availability: **%s**",
					SandC.CourseName, SandC.Nickname, SandC.Availability)).
				AddActionRow(
					discord.NewPrimaryButton("Approve", SandCApprovePrefix+SandC.UserID),
					discord.NewDangerButton("Deny", SandCDenyPrefix+SandC.UserID),
				).
				Build(),
		)
		if err != nil {
			slog.Error("error while creating approval message", slog.Any("err", err))
		}

		dmChannel, err := event.Client().Rest().CreateDMChannel(event.User().ID)
		if err != nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your school or course request has been **submitted**.").
					Build(),
			)
		}

		_ = event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContent("✅ Your school or course request has been submitted. You will receive updates via DM.").
			Build())
	}
})

var SandCApprovalButtonListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	if strings.HasPrefix(customID, SandCApprovePrefix) {
		userID := strings.TrimPrefix(customID, SandCApprovePrefix)

		SandCRequestsMutex.Lock()
		for i := range SandCRequests {
			if SandCRequests[i].UserID == userID {
				approved := true
				SandCRequests[i].Approved = &approved
				SandCRequests[i].ReviewedBy = event.User().Tag()
				break
			}
		}
		SandCRequestsMutex.Unlock()

		updateSandCForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContent("✅ Your school or course request has been **approved**.").
					Build(),
			)
		}

		_ = event.Client().Rest().DeleteMessage(SandCApprovalChannelID, event.Message.ID)

	} else if strings.HasPrefix(customID, SandCDenyPrefix) {
		userID := strings.TrimPrefix(customID, SandCDenyPrefix)

		textInput := discord.NewTextInput("deny-reason", discord.TextInputStyleParagraph, "Reason for denial (required)")
		textInput.Required = true
		maxLength := 400
		textInput.MaxLength = maxLength

		err := event.Modal(
			discord.NewModalCreateBuilder().
				SetTitle("Deny School or Course Request").
				SetCustomID("sandc-deny-modal:" + userID).
				AddActionRow(textInput).
				Build(),
		)
		if err != nil {
			slog.Error("failed to show deny modal", slog.Any("err", err))
		}
	}
})

var SandCDenyModalListener = bot.NewListenerFunc(func(event *events.ModalSubmitInteractionCreate) {
	if strings.HasPrefix(event.Data.CustomID, SandCDenyModalPrefix) {
		userID := strings.TrimPrefix(event.Data.CustomID, SandCDenyModalPrefix)
		reason, _ := event.Data.TextInputComponent("deny-reason")

		SandCRequestsMutex.Lock()
		for i := range SandCRequests {
			if SandCRequests[i].UserID == userID {
				approved := false
				SandCRequests[i].Approved = &approved
				SandCRequests[i].ReviewedBy = event.User().Tag()
				SandCRequests[i].DenyReason = reason.Value
				break
			}
		}
		SandCRequestsMutex.Unlock()

		updateSandCForumPost(event.Client())

		dmChannel, err := event.Client().Rest().CreateDMChannel(snowflake.MustParse(userID))
		if err == nil {
			_, _ = event.Client().Rest().CreateMessage(dmChannel.ID(),
				discord.NewMessageCreateBuilder().
					SetContentf("❌ Your school or course request has been **denied**.\n**Reason:** %s", reason.Value).
					Build(),
			)
		}

		_ = event.CreateMessage(discord.NewMessageCreateBuilder().
			SetEphemeral(true).
			SetContent("School or Course request denied and user notified.").
			Build())

		_ = event.Client().Rest().DeleteMessage(SandCApprovalChannelID, event.Message.ID)
	}
})

var SandCListCommandListener = bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() == "school-list" {
		SandCRequestsMutex.Lock()
		defer SandCRequestsMutex.Unlock()
		if len(SandCRequests) == 0 {
			event.CreateMessage(discord.NewMessageCreateBuilder().
				SetContent("No current requests.").
				Build())
			return
		}

		var sb strings.Builder
		//var allNicknames []string
		sb.WriteString("**Current School and Course Requests:**\n")
		for _, req := range SandCRequests {
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
					"\n• %s submitted by **%s** at %s with availability of: **%s** (Status: %s)\n",
					req.CourseName,
					req.Nickname,
					req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
					req.Availability,
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

func SandCClearCommandListener(event *events.ApplicationCommandInteractionCreate) {
	if event.Data.CommandName() != "school-clear" {
		return
	}

	//slog.Info("SandC-CLEAR command triggered")

	userNameToClear := ""

	// Extract the nickname option if it's present
	if data, ok := event.Data.(discord.SlashCommandInteractionData); ok {
		//slog.Info("SandC-CLEAR options", slog.Any("options", data.Options))
		for _, opt := range data.Options {
			//slog.Info("SandC-CLEAR option", slog.String("name", opt.Name), slog.String("type", fmt.Sprintf("%v", opt.Type)), slog.Any("value", opt.Value))
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

	//slog.Info("Proceeding to SandC clear logic", slog.String("userNameToClear", userNameToClear))

	SandCRequestsMutex.Lock()
	defer SandCRequestsMutex.Unlock()

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

		for i, SandC := range SandCRequests {
			nickLower := strings.ToLower(SandC.Nickname)

			// Check exact match on full nickname
			if nickLower == userInputLower {
				SandCRequests = append(SandCRequests[:i], SandCRequests[i+1:]...)
				found = true
				break
			}

			// Otherwise, fuzzy match on last word
			parts := strings.Fields(SandC.Nickname)
			if len(parts) > 0 {
				lastWord := strings.ToLower(parts[len(parts)-1])
				if strings.Contains(lastWord, userInputLower) {
					SandCRequests = append(SandCRequests[:i], SandCRequests[i+1:]...)
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

func updateSandCForumPost(client bot.Client) {
	SandCRequestsMutex.Lock()
	defer SandCRequestsMutex.Unlock()

	var newEntries strings.Builder

	for _, req := range SandCRequests {
		if req.Approved == nil {
			continue
		}

		// More stable and readable key
		key := fmt.Sprintf("%s|%s|%v", req.UserID, req.Availability, req.Approved)

		SandCForumAddedMutex.Lock()
		if SandCForumAdded[key] {
			SandCForumAddedMutex.Unlock()
			continue
		}
		SandCForumAdded[key] = true
		SandCForumAddedMutex.Unlock()

		status := "🕐 pending"
		if *req.Approved {
			status = "✅ approved by " + req.ReviewedBy
		} else {
			status = fmt.Sprintf("❌ denied by %s — Reason: **%s**", req.ReviewedBy, req.DenyReason)
		}

		newEntries.WriteString(fmt.Sprintf(
			"\n• %s submitted by **%s** at %s with availability of: **%s** (Status: %s)\n",
			req.CourseName,
			req.Nickname,
			req.Submitted.In(time.FixedZone("CST", -6*60*60)).Format("Mon, 02 Jan 2006 15:04 MST"),
			req.Availability,
			status,
		))
	}

	if newEntries.Len() == 0 {
		return // Nothing new to add
	}

	if SandCForumMessageID == 0 {
		// Create new summary post
		msg, err := client.Rest().CreateMessage(SandCForumThreadID,
			discord.NewMessageCreateBuilder().
				SetContent("**Schools and Courses Log:**\n"+newEntries.String()).
				Build())
		if err != nil {
			slog.Error("failed to create SandC forum post", slog.Any("err", err))
			return
		}
		SandCForumMessageID = msg.ID
	} else {
		// Append to existing message
		msg, err := client.Rest().GetMessage(SandCForumThreadID, SandCForumMessageID)
		if err != nil {
			slog.Error("failed to fetch SandC forum post", slog.Any("err", err))
			return
		}

		newContent := msg.Content + newEntries.String()
		if len(newContent) > 2000 {
			newContent = newContent[:2000] // truncate to Discord limit
		}

		_, err = client.Rest().UpdateMessage(SandCForumThreadID, SandCForumMessageID,
			discord.NewMessageUpdateBuilder().SetContent(newContent).Build())
		if err != nil {
			slog.Error("failed to update SandC forum post", slog.Any("err", err))
		}
	}
}
