package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmorganca/ollama/format"
	"golang.org/x/term"
)

type Bar struct {
	message      string
	messageWidth int

	maxValue     int64
	initialValue int64
	currentValue int64

	started time.Time
	stopped time.Time

	maxBuckets int
	buckets    []bucket
}

type bucket struct {
	updated time.Time
	value   int64
}

func NewBar(message string, maxValue, initialValue int64) *Bar {
	return &Bar{
		message:      message,
		messageWidth: -1,
		maxValue:     maxValue,
		initialValue: initialValue,
		currentValue: initialValue,
		started:      time.Now(),
		maxBuckets:   10,
	}
}

// formatDuration limits the rendering of a time.Duration to 2 units
func formatDuration(d time.Duration) string {
	switch {
	case d >= 100*time.Hour:
		return "99h+"
	case d >= time.Hour:
		return d.Round(time.Minute).String()
	default:
		return d.Round(time.Second).String()
	}
}

func (b *Bar) String() string {
	termWidth, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		termWidth = 80
	}

	var pre strings.Builder
	if len(b.message) > 0 {
		message := strings.TrimSpace(b.message)
		if b.messageWidth > 0 && len(message) > b.messageWidth {
			message = message[:b.messageWidth]
		}

		fmt.Fprintf(&pre, "%s", message)
		if padding := b.messageWidth - pre.Len(); padding > 0 {
			pre.WriteString(strings.Repeat(" ", padding))
		}

		pre.WriteString(" ")
	}

	fmt.Fprintf(&pre, "%3.0f%%", b.percent())

	var suf strings.Builder
	// max 13 characters: "999 MB/999 MB"
	if b.stopped.IsZero() {
		curValue := format.HumanBytes(b.currentValue)
		suf.WriteString(strings.Repeat(" ", 6-len(curValue)))
		suf.WriteString(curValue)
		suf.WriteString("/")

		maxValue := format.HumanBytes(b.maxValue)
		suf.WriteString(strings.Repeat(" ", 6-len(maxValue)))
		suf.WriteString(maxValue)
	} else {
		maxValue := format.HumanBytes(b.maxValue)
		suf.WriteString(strings.Repeat(" ", 6-len(maxValue)))
		suf.WriteString(maxValue)
		suf.WriteString(strings.Repeat(" ", 7))
	}

	rate := b.rate()
	// max 10 characters: ", 999 MB/s"
	if b.stopped.IsZero() {
		suf.WriteString(", ")
		humanRate := format.HumanBytes(int64(rate))
		suf.WriteString(strings.Repeat(" ", 6-len(humanRate)))
		suf.WriteString(humanRate)
		suf.WriteString("/s")
	} else {
		suf.WriteString(strings.Repeat(" ", 10))
	}

	// max 8 characters: ", 59m59s"
	if b.stopped.IsZero() {
		suf.WriteString(", ")
		var remaining time.Duration
		if rate > 0 {
			remaining = time.Duration(int64(float64(b.maxValue-b.currentValue)/rate)) * time.Second
		}

		humanRemaining := formatDuration(remaining)
		suf.WriteString(strings.Repeat(" ", 6-len(humanRemaining)))
		suf.WriteString(humanRemaining)
	} else {
		suf.WriteString(strings.Repeat(" ", 8))
	}

	var mid strings.Builder
	// add 3 extra spaces: 2 boundary characters and 1 space at the end
	f := termWidth - pre.Len() - suf.Len() - 3
	n := int(float64(f) * b.percent() / 100)

	mid.WriteString("▕")

	if n > 0 {
		mid.WriteString(strings.Repeat("█", n))
	}

	if f-n > 0 {
		mid.WriteString(strings.Repeat(" ", f-n))
	}

	mid.WriteString("▏")

	return pre.String() + mid.String() + suf.String()
}

func (b *Bar) Set(value int64) {
	if value >= b.maxValue {
		value = b.maxValue
	}

	b.currentValue = value
	if b.currentValue >= b.maxValue {
		b.stopped = time.Now()
	}

	// throttle bucket updates to 1 per second
	if len(b.buckets) == 0 || time.Since(b.buckets[len(b.buckets)-1].updated) > time.Second {
		b.buckets = append(b.buckets, bucket{
			updated: time.Now(),
			value:   value,
		})

		if len(b.buckets) > b.maxBuckets {
			b.buckets = b.buckets[1:]
		}
	}
}

func (b *Bar) percent() float64 {
	if b.maxValue > 0 {
		return float64(b.currentValue) / float64(b.maxValue) * 100
	}

	return 0
}

func (b *Bar) rate() float64 {
	if !b.stopped.IsZero() {
		elapsed := b.stopped.Sub(b.started).Round(time.Second)
		return (float64(b.currentValue) - float64(b.initialValue)) / elapsed.Seconds()
	}

	switch len(b.buckets) {
	case 0:
		return 0
	case 1:
		return float64(b.buckets[0].value-b.initialValue) / b.buckets[0].updated.Sub(b.started).Seconds()
	default:
		first, last := b.buckets[0], b.buckets[len(b.buckets)-1]
		return (float64(last.value) - float64(first.value)) / last.updated.Sub(first.updated).Seconds()
	}
}
