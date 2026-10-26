package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var ErrConfigurationChanged = errors.New("配置文件已变化，请重新载入或重启后再保存")

// ConfigurationGeneration is local process state, never a shared field.
func (s *Store) ConfigurationGeneration() uint64 { return s.configurationGeneration.Load() }

func configurationDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *Store) observeConfiguration(data []byte, config ...Config) {
	if len(config) > 0 {
		value := config[0].Clone()
		s.baseline = &value
	}
	if !s.observed || s.observedRevision != configurationDigest(data) {
		s.configurationGeneration.Add(1)
	}
	s.observed = true
	s.observedRevision = configurationDigest(data)
}
func (s *Store) checkConfigurationRevision(expected string) error {
	current, err := s.Revision()
	if err != nil {
		return err
	}
	if current != expected || s.observed && s.observedRevision != expected {
		return ErrConfigurationChanged
	}
	return nil
}

func (s *Store) ConfigurationBaseline() (Config, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseline == nil {
		return Config{}, "", fmt.Errorf("没有可用的本地配置基线")
	}
	return s.baseline.Clone(), s.observedRevision, nil
}
