// Package logo renders a Toke wordmark in a stylized way.
package logo

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/slice"
)

// letterform represents a letterform. It can be stretched horizontally by
// a given amount via the boolean argument.
type letterform func(bool) string

const diag = `🍃` // Changed to weed leaf emoji

// Opts are the options for rendering the Toke title art.
type Opts struct {
	FieldColor   color.Color // diagonal lines (now weed leaves)
	TitleColorA  color.Color // left gradient ramp point (green)
	TitleColorB  color.Color // right gradient ramp point (lighter green)
	CharmColor   color.Color // Weedmaps™ text color
	VersionColor color.Color // Version text color
	Width        int         // width of the rendered logo, used for truncation
}

// Render renders the Toke logo. Set the argument to true to render the narrow
// version, intended for use in a sidebar.
//
// The compact argument determines whether it renders compact for the sidebar
// or wider for the main pane.
func Render(version string, compact bool, o Opts) string {
	const charm = " Weedmaps™"

	fg := func(c color.Color, s string) string {
		return lipgloss.NewStyle().Foreground(c).Render(s)
	}

	// Title.
	const spacing = 1
	letterforms := []letterform{
		letterT,
		letterO,
		letterK,
		letterE,
	}
	stretchIndex := -1 // -1 means no stretching.
	if !compact {
		stretchIndex = rand.IntN(len(letterforms))
	}

	toke := renderWord(spacing, stretchIndex, letterforms...)
	tokeWidth := lipgloss.Width(toke)
	b := new(strings.Builder)
	for r := range strings.SplitSeq(toke, "\n") {
		fmt.Fprintln(b, styles.ApplyForegroundGrad(r, o.TitleColorA, o.TitleColorB))
	}
	toke = b.String()

	// Weedmaps and version.
	metaRowGap := 1
	maxVersionWidth := tokeWidth - lipgloss.Width(charm) - metaRowGap
	version = ansi.Truncate(version, maxVersionWidth, "…") // truncate version if too long.
	gap := max(0, tokeWidth-lipgloss.Width(charm)-lipgloss.Width(version))
	metaRow := fg(o.CharmColor, charm) + strings.Repeat(" ", gap) + fg(o.VersionColor, version)

	// Join the meta row and big Toke title.
	toke = strings.TrimSpace(metaRow + "\n" + toke)

	// Narrow version.
	if compact {
		field := fg(o.FieldColor, strings.Repeat(diag, tokeWidth/2)) // Adjusted for emoji width
		return strings.Join([]string{field, field, toke, field, ""}, "\n")
	}

	fieldHeight := lipgloss.Height(toke)

	// Left field (with weed leaves).
	const leftWidth = 3
	leftFieldRow := fg(o.FieldColor, strings.Repeat(diag, leftWidth))
	leftField := new(strings.Builder)
	for range fieldHeight {
		fmt.Fprintln(leftField, leftFieldRow)
	}

	// Right field (with weed leaves).
	rightWidth := max(7, (o.Width-tokeWidth-leftWidth*2-2)/2) // Adjusted for emoji width
	const stepDownAt = 0
	rightField := new(strings.Builder)
	for i := range fieldHeight {
		width := rightWidth
		if i >= stepDownAt {
			width = max(1, rightWidth-(i-stepDownAt))
		}
		fmt.Fprint(rightField, fg(o.FieldColor, strings.Repeat(diag, width)), "\n")
	}

	// Return the wide version.
	const hGap = " "
	logo := lipgloss.JoinHorizontal(lipgloss.Top, leftField.String(), hGap, toke, hGap, rightField.String())
	if o.Width > 0 {
		// Truncate the logo to the specified width.
		lines := strings.Split(logo, "\n")
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, o.Width, "")
		}
		logo = strings.Join(lines, "\n")
	}
	return logo
}

// SmallRender renders a smaller version of the Toke logo, suitable for
// smaller windows or sidebar usage.
func SmallRender(width int) string {
	t := styles.CurrentTheme()
	title := t.S().Base.Foreground(t.Secondary).Render("Weedmaps™")
	title = fmt.Sprintf("%s %s", title, styles.ApplyBoldForegroundGrad("Toke", t.Secondary, t.Primary))
	remainingWidth := width - lipgloss.Width(title) - 1 // 1 for the space after "Toke"
	if remainingWidth > 0 {
		lines := strings.Repeat("🍃", remainingWidth/2) // Adjusted for emoji width
		title = fmt.Sprintf("%s %s", title, t.S().Base.Foreground(t.Primary).Render(lines))
	}
	return title
}

// renderWord renders letterforms to fork a word. stretchIndex is the index of
// the letter to stretch, or -1 if no letter should be stretched.
func renderWord(spacing int, stretchIndex int, letterforms ...letterform) string {
	if spacing < 0 {
		spacing = 0
	}

	renderedLetterforms := make([]string, len(letterforms))

	// pick one letter randomly to stretch
	for i, letter := range letterforms {
		renderedLetterforms[i] = letter(i == stretchIndex)
	}

	if spacing > 0 {
		// Add spaces between the letters and render.
		renderedLetterforms = slice.Intersperse(renderedLetterforms, strings.Repeat(" ", spacing))
	}
	return strings.TrimSpace(
		lipgloss.JoinHorizontal(lipgloss.Top, renderedLetterforms...),
	)
}

// letterT renders the letter T in a stylized way.
func letterT(stretch bool) string {
	// Here's what we're making:
	//
	// ▀▀▀▀▀
	//   █
	//   ▀
	
	top := heredoc.Doc(`
		▀
	`)
	middle := heredoc.Doc(`
		█
		▀
	`)
	return joinLetterform(
		stretchLetterformPart(top, letterformProps{
			stretch:    stretch,
			width:      5,
			minStretch: 7,
			maxStretch: 12,
		}),
		"\n"+middle,
	)
}

// letterO renders the letter O in a stylized way.
func letterO(stretch bool) string {
	// Here's what we're making:
	//
	// ▄▀▀▀▄
	// █   █
	// ▀▀▀▀
	
	left := heredoc.Doc(`
		▄
		█
		▀
	`)
	middle := heredoc.Doc(`
		▀
		
		▀
	`)
	right := heredoc.Doc(`
		▄
		█
		▀
	`)
	return joinLetterform(
		left,
		stretchLetterformPart(middle, letterformProps{
			stretch:    stretch,
			width:      3,
			minStretch: 6,
			maxStretch: 10,
		}),
		right,
	)
}

// letterK renders the letter K in a stylized way.
func letterK(stretch bool) string {
	// Here's what we're making:
	//
	// █  ▄
	// █▀▀
	// ▀  ▀
	
	left := heredoc.Doc(`
		█
		█
		▀
	`)
	middle := heredoc.Doc(`
		
		▀
	`)
	right := heredoc.Doc(`
		▄
		
		▀
	`)
	return joinLetterform(
		left,
		stretchLetterformPart(middle, letterformProps{
			stretch:    stretch,
			width:      2,
			minStretch: 4,
			maxStretch: 8,
		}),
		right,
	)
}

// letterE renders the letter E in a stylized way.
func letterE(stretch bool) string {
	// Here's what we're making:
	//
	// ▄▀▀▀
	// █▀▀
	// ▀▀▀▀
	
	left := heredoc.Doc(`
		▄
		█
		▀
	`)
	right := heredoc.Doc(`
		▀
		▀
		▀
	`)
	return joinLetterform(
		left,
		stretchLetterformPart(right, letterformProps{
			stretch:    stretch,
			width:      3,
			minStretch: 6,
			maxStretch: 10,
		}),
	)
}

func joinLetterform(letters ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, letters...)
}

// letterformProps defines letterform stretching properties.
// for readability.
type letterformProps struct {
	width      int
	minStretch int
	maxStretch int
	stretch    bool
}

// stretchLetterformPart is a helper function for letter stretching. If randomize
// is false the minimum number will be used.
func stretchLetterformPart(s string, p letterformProps) string {
	if p.maxStretch < p.minStretch {
		p.minStretch, p.maxStretch = p.maxStretch, p.minStretch
	}
	n := p.width
	if p.stretch {
		n = rand.IntN(p.maxStretch-p.minStretch) + p.minStretch //nolint:gosec
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = s
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}