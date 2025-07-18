package perscom_events

import (
	_ "embed"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
	"strings"
)

const schoolAndCourseRequestCustomID = "school-and-course-request"
const selectedCourseCustomID = "selected-course"
const selectedCourseAvailabilityModalSubmit = "selected-course-availability-modal-submit"

//go:embed school_and_course_request_description.txt
var schoolAndCourseDescription string

var schoolAndCourseRequest = ButtonEventHandler{
	discord.NewPrimaryButton("Schools & Courses", schoolAndCourseRequestCustomID),
	[]bot.EventListener{schoolAndCourseRequestEventListener, schoolAndCourseRequestSelectionEventListener, schoolAndCourseModalSubmitEventListener},
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
					discord.NewStringSelectMenuOption("Air Assault", "Air assault"),
					discord.NewStringSelectMenuOption("Advanced Infantry Training", "Advanced Infantry Training"),
					discord.NewStringSelectMenuOption("Ranger School", "Ranger School"),
					discord.NewStringSelectMenuOption("Combat Life Saver", "Combat Life Saver"),
					discord.NewStringSelectMenuOption("Drill Instructor Course", "Drill Instructor Course"),
					discord.NewStringSelectMenuOption("NCO Training & Leadership", "NCO Training & Leadership"),
					discord.NewStringSelectMenuOption("Squad Designated Marksman (SDM)", "Squad Designated Marksman (SDM)"),
					discord.NewStringSelectMenuOption("Explosive Ordnance Disposal (EOD)", "Explosive Ordnance Disposal (EOD)"),
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

		// TODO: Grab availability from req and make forum post
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
