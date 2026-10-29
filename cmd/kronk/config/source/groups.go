package source

import "strings"

// Group is where the picker lists a model.
type Group string

// The picker's groups.
const (
	GroupChat       Group = "chat"
	GroupEmbeddings Group = "embeddings"
	GroupRerank     Group = "rerank"
	GroupDecision   Group = "decision"
	GroupAudio      Group = "audio"
	GroupImage      Group = "image"
)

// Groups are the picker's groups in the order they are listed.
var Groups = []Group{GroupChat, GroupEmbeddings, GroupRerank, GroupDecision, GroupAudio, GroupImage}

// Group is the picker group of c. A model that doesn't generate text is a
// reranker when its name says so, the rule the runtime uses to load it with
// rank pooling, and an embedding model otherwise. The catalog's endpoint
// can't tell the two apart: it calls every BERT model a reranker.
func (c Choice) Group() Group {
	switch {
	case c.Kind == KindAudioModel:
		return GroupAudio
	case c.Kind == KindImageModel:
		return GroupImage
	case c.Endpoint == "":
		return GroupChat
	case c.Endpoint == "decision":
		return GroupDecision
	case strings.Contains(strings.ToLower(c.ID), "rerank"):
		return GroupRerank
	}
	return GroupEmbeddings
}
