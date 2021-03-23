package pipemap

import (
	"bufio"
	"io"
)

const BufferSize = 1024

// Buffer the reader, map over strings, and pipe them to the writer, all in a
// background thread.
//
// The receive-only channel returned will fire once all data has been piped.
func Strings(reader io.Reader, writer io.Writer, mapper func(string) string) <-chan bool {
	bufReader := bufio.NewReader(reader)
	runeCh := make(chan rune, BufferSize)
	doneCh := make(chan bool)
	go consumeRunes(bufReader, runeCh)
	go mapRunes(writer, runeCh, mapper, doneCh)
	return doneCh
}

func consumeRunes(bufReader *bufio.Reader, runeCh chan<- rune) {
	for {
		r, size, err := bufReader.ReadRune()
		if size > 0 {
			runeCh <- r
		}

		if err != nil {
			if err == io.EOF {
				close(runeCh)
				return
			} else {
				panic(err.Error())
			}
		}
	}
}

func mapRunes(writer io.Writer, runeCh <-chan rune, mapper func(string) string, doneCh chan<- bool) {
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
		r, more := <-runeCh
		if !more {
			break STREAM_LOOP
		}

		buffer = append(buffer, r)

		// Loop to buffer a chunk
	BUFFER_LOOP:
		for {
			// Nonblocking read
			select {
			case r, more = <-runeCh:
				if more {
					buffer = append(buffer, r)
				} else {
					// The stream is done, so stop completely
					dispatchCurrentBuffer()
					break STREAM_LOOP
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
