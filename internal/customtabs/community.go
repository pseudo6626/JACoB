package customtabs

import (
	"errors"
	"strings"
)

type CommunityOrigin struct {
	PackageID string `json:"packageId"`
	Version   string `json:"version"`
}

type CommunityInfo struct {
	PackageID            string           `json:"packageId"`
	PublishedVersion     string           `json:"publishedVersion,omitempty"`
	LastSubmittedVersion string           `json:"lastSubmittedVersion,omitempty"`
	PublishedSHA256      string           `json:"publishedSha256,omitempty"`
	Registry             string           `json:"registry,omitempty"`
	Author               string           `json:"author,omitempty"`
	Description          string           `json:"description,omitempty"`
	Category             string           `json:"category,omitempty"`
	Tags                 []string         `json:"tags,omitempty"`
	BasedOn              *CommunityOrigin `json:"basedOn,omitempty"`
	LocalAuthor          bool             `json:"localAuthor,omitempty"`
	Status               string           `json:"status,omitempty"`
}

func (s *Store) SetCommunity(id string, info *CommunityInfo) (Tab, error) {
	id = strings.TrimSpace(id)
	if !validTabID(id) {
		return Tab{}, errors.New("invalid saved tab id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	old, ok := s.tabs[id]
	if !ok {
		return Tab{}, errors.New("saved tab not found")
	}
	t := old
	t.Community = cloneCommunityInfo(info)
	s.tabs[id] = t
	if err := s.persistLocked(); err != nil {
		s.tabs[id] = old
		return Tab{}, err
	}
	return t, nil
}

func cloneCommunityInfo(in *CommunityInfo) *CommunityInfo {
	if in == nil {
		return nil
	}
	out := *in
	out.Tags = append([]string(nil), in.Tags...)
	if in.BasedOn != nil {
		origin := *in.BasedOn
		out.BasedOn = &origin
	}
	return &out
}
