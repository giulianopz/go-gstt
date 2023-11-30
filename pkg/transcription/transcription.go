package transcription

type Response struct {
	Result []struct {
		Alternative []struct {
			Transcript string  `json:"transcript,omitempty"`
			Confidence float64 `json:"confidence,omitempty"`
		} `json:"alternative,omitempty"`
		Final bool `json:"final,omitempty"`
	} `json:"result,omitempty"`
	ResultIndex int `json:"result_index,omitempty"`
}
