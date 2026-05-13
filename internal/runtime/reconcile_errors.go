package runtime

import "fmt"

type ErrVisibleRecordAmbiguous struct {
	Output string
	Key    string
	Count  int
}

func (e *ErrVisibleRecordAmbiguous) Error() string {
	return fmt.Sprintf("visible record ambiguity for output %s key %s: %d matches", e.Output, e.Key, e.Count)
}
