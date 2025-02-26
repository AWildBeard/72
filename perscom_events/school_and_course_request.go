package perscom_events

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"log/slog"
)

const schoolAndCourseRequestCustomID = "school-and-course-request"
const selectedCourseCustomID = "selected-course"

var schoolAndCourseRequest = ButtonEventHandler{
	discord.NewPrimaryButton("School & Course Request", schoolAndCourseRequestCustomID),
	[]bot.EventListener{schoolAndCourseRequestEventListener, schoolAndCourseRequestSelectionEventListener},
}

var schoolAndCourseRequestEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == schoolAndCourseRequestCustomID {
		err := event.CreateMessage(
			discord.NewMessageCreateBuilder().
				SetEphemeral(true).
				SetEmbeds(discord.NewEmbedBuilder().
					SetTitle("School & Course Descriptions").
					SetDescription("See the descriptions here: [72nd Airborne Schools and Courses](https://72ndairborne.com/ipbdev/schools-and-courses/)").
					SetColor(0x00FF00). // Example color
					Build()).
				AddActionRow(discord.NewStringSelectMenu(selectedCourseCustomID, "Select a school or course",
					discord.NewStringSelectMenuOption("Airborne", "Airborne"),
					discord.NewStringSelectMenuOption("Air Assault", "Air assault"),
					discord.NewStringSelectMenuOption("Advanced Infantry Training", "Advanced Infantry Training"),
					discord.NewStringSelectMenuOption("Ranger School", "Ranger School"),
					discord.NewStringSelectMenuOption("Combat Life Saver", "Combat Life Saver"),
					discord.NewStringSelectMenuOption("Drill Instructor Course", "Drill Instructor Course"),
					discord.NewStringSelectMenuOption("NCO Training & Leadership", "NCO Training & Leadership"))).
				Build(),
		)

		if err != nil {
			slog.Error("error while creating message", slog.Any("err", err))
		}
	}
})

var schoolAndCourseRequestSelectionEventListener = bot.NewListenerFunc(func(event *events.ComponentInteractionCreate) {
	if event.Data.CustomID() == selectedCourseCustomID {
		err := event.UpdateMessage(discord.NewMessageUpdateBuilder().
			ClearEmbeds().
			ClearContainerComponents().
			SetContentf("Submitted your request for %v.", event.StringSelectMenuInteractionData().Values[0]).
			Build())

		if err != nil {
			slog.Error("error while updating message", slog.Any("err", err))
		}
	}
})
