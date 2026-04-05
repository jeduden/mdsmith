package conciseness

// ConcisenessEvalRecord defines one JSONL line for evaluation.
#ConcisenessEvalRecord: {
	id:       string
	file:     string
	doc_type: string
	paragraph: string & !=""
	label: #Label

	annotator_a: #Label
	annotator_b: #Label

	line?: int & >=1

	cues?: [...#Cue]
	cue_summary?: string

	adjudication_note?: string
	split?: #Split

	// Free-form optional metadata.
	metadata?: {[string]: _}

	// Reject unknown fields.
	..._|_
}

#Label: "verbose-actionable" | "acceptable"

#Split:
	"train" |
	"dev" |
	"test" |
	"holdout-outofdomain"

#CueKind:
	"filler" |
	"hedge" |
	"verbose-phrase" |
	"redundancy" |
	"other"

#CueSource:
	"annotator" |
	"heuristic" |
	"classifier" |
	"adjudicated"

#Cue: {
	text: string & !=""
	kind: #CueKind

	source?: #CueSource
	start_token?: int & >=0
	end_token?: int & >=0

	..._|_
}
