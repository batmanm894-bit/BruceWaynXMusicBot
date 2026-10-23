package database

// DisabledPlatforms returns the (lowercase) names of platforms the owner
// switched off.
func DisabledPlatforms() ([]string, error) {
	s, err := getBotState()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(s.DisabledPlatforms))
	copy(out, s.DisabledPlatforms)
	return out, nil
}

// SetDisabledPlatforms replaces the whole off-list.
func SetDisabledPlatforms(names []string) error {
	return modifyBotState(func(s *BotState) bool {
		s.DisabledPlatforms = append([]string{}, names...)
		return true
	})
}
