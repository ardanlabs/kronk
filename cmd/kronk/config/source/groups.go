package source

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

// Group is the picker group of c, from its endpoint.
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
	case c.Endpoint == "rerank":
		return GroupRerank
	}
	return GroupEmbeddings
}
