package pipemap

import (
	"bufio"
	"io"
)

type RuneStatus struct {
	Rune rune
	EOF  bool
}

const BufferSize = 1024

// Buffer the reader, map over strings, and pipe them to the writer, all in a
// background thread.
//
// The receive-only channel returned will fire once all data has been piped.
func Strings(reader io.Reader, writer io.Writer, mapper func(string) string) <-chan bool {
	bufReader := bufio.NewReader(reader)
	runeCh := make(chan RuneStatus, BufferSize)
	doneCh := make(chan bool)
	go consumeRunes(bufReader, runeCh)
	go mapRunes(writer, runeCh, mapper, doneCh)
	return doneCh
}

func consumeRunes(bufReader *bufio.Reader, runeCh chan<- RuneStatus) {
	for {
		r, size, err := bufReader.ReadRune()
		if size > 0 {
			runeCh <- RuneStatus{r, false}
		}

		if err != nil {
			if err == io.EOF {
				runeCh <- RuneStatus{'\x00', true}
				return
			} else {
				panic(err.Error())
			}
		}
	}
}

func mapRunes(writer io.Writer, runeCh <-chan RuneStatus, mapper func(string) string, doneCh chan<- bool) {
	defer func() { doneCh <- true }()

	buffer := make([]rune, 0, BufferSize)
	done := false

	dispatchCurrentBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		mappedString := mapper(string(buffer))
		io.WriteString(writer, mappedString)
		// Truncating means that appends won't reallocate
		buffer = buffer[:0]
	}

	// Loop over the entire incoming data
STREAM_LOOP:
	for !done {
		// Block for first piece
		runeStatus := <-runeCh
		if runeStatus.EOF {
			break STREAM_LOOP
		}

		buffer = append(buffer, runeStatus.Rune)

		// Loop to buffer a chunk
	BUFFER_LOOP:
		for {
			// Nonblocking read
			select {
			case runeStatus = <-runeCh:
				if runeStatus.EOF {
					// The stream is done, so stop completely
					dispatchCurrentBuffer()
					break STREAM_LOOP
				} else {
					// Push the rune into the current buffer
					buffer = append(buffer, runeStatus.Rune)
				}
			default:
				// Since the channel blocked, dispatch what we have, then stop consuming
				// until the next block
				dispatchCurrentBuffer()
				break BUFFER_LOOP
			}
		}
	}
}
