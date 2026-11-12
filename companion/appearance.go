package main

import (
	"fmt"
	"net/http"
)

type appearance struct {
	Animate     bool `json:"animate"`
	DimWhenIdle bool `json:"dimWhenIdle"`
	// Hidden is the shell's state, never saved: a restart starts on the screen.
	Hidden bool `json:"hidden"`
}

func (s settings) appearance() appearance {
	return appearance{Animate: s.animate(), DimWhenIdle: s.DimWhenIdle}
}

func (s *shell) appearance() appearance {
	s.mu.Lock()
	defer s.mu.Unlock()
	looks := s.settings.appearance()
	looks.Hidden = s.hidden
	return looks
}

// setAnimate saves whether the pages move.
func (s *shell) setAnimate(on bool) error {
	if err := s.changeLook(func(saved *settings) { saved.Animate = &on }); err != nil {
		return fmt.Errorf("save the animation choice: %w", err)
	}
	return nil
}

// setDimWhenIdle saves whether the circle fades while nothing needs the person.
func (s *shell) setDimWhenIdle(on bool) error {
	if err := s.changeLook(func(saved *settings) { saved.DimWhenIdle = on }); err != nil {
		return fmt.Errorf("save the dim choice: %w", err)
	}
	return nil
}

// changeLook saves a change to the settings and tells the pages the
// appearance in effect: the new one, or the old one when the save failed.
func (s *shell) changeLook(change func(*settings)) error {
	s.mu.Lock()
	before := s.settings
	change(&s.settings)
	err := writeSettings(s.settingsPath, s.settings)
	if err != nil {
		s.settings = before
	}
	looks := s.settings.appearance()
	looks.Hidden = s.hidden
	s.mu.Unlock()
	s.tell(looks)
	s.refreshNative()
	return err
}

func (s *shell) getAppearance(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.appearance())
}
