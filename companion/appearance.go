package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// bubbleMenuName is the circle's right-click menu; bubble.css names it in
// --custom-contextmenu.
const bubbleMenuName = "bubble"

type appearance struct {
	Animate bool `json:"animate"`
}

func (s *shell) appearance() appearance {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appearance{Animate: s.settings.animate()}
}

// setAnimate saves whether the pages move and tells them the appearance
// in effect: the new one, or the old one when the save failed.
func (s *shell) setAnimate(on bool) error {
	s.mu.Lock()
	before := s.settings.Animate
	s.settings.Animate = &on
	err := writeSettings(s.settingsPath, s.settings)
	if err != nil {
		s.settings.Animate = before
	}
	looks := appearance{Animate: s.settings.animate()}
	s.mu.Unlock()
	s.tell(looks)
	if err != nil {
		return fmt.Errorf("save the animation choice: %w", err)
	}
	return nil
}

func (s *shell) getAppearance(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.appearance())
}

func (s *shell) bubbleMenu() {
	menu := application.NewContextMenu(bubbleMenuName)
	menu.AddSectionHeader("Settings")
	animate := menu.AddCheckbox("Animate companion", s.appearance().Animate)
	animate.OnClick(func(clicked *application.Context) {
		if err := s.setAnimate(clicked.IsChecked()); err != nil {
			log.Print(err)
			animate.SetChecked(s.appearance().Animate)
		}
	})
}
