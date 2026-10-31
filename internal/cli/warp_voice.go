package cli

func warpVoiceLanguage(language string) bool {
	return language == "" || len(language) == 2 && language[0] >= 'a' && language[0] <= 'z' && language[1] >= 'a' && language[1] <= 'z'
}
