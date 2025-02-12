package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/sst/ion/pkg/project"
)

type Progress struct {
	Color   color.Attribute
	Label   string
	URN     string
	Final   bool
	Message string
	time.Duration
}

type ProgressMode string

const (
	ProgressModeDeploy  ProgressMode = "deploy"
	ProgressModeRemove  ProgressMode = "remove"
	ProgressModeCancel  ProgressMode = "cancel"
	ProgressModeRefresh ProgressMode = "refresh"
)

func progress(mode ProgressMode, events project.StackEventStream) bool {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	pending := map[string]string{}
	if mode == ProgressModeRemove {
		spin.Suffix = "  Removing..."
	}
	if mode == ProgressModeDeploy {
		spin.Suffix = "  Deploying..."
	}
	if mode == ProgressModeCancel {
		spin.Suffix = "  Cancelling..."
	}
	if mode == ProgressModeRefresh {
		spin.Suffix = "  Refreshing..."
	}
	spin.Start()
	defer spin.Stop()

	formatURN := func(urn string) string {
		splits := strings.Split(urn, "::")[2:]
		urn0 := splits[0]
		resourceName0 := splits[1]
		// convert aws:s3/bucket:Bucket to aws:s3:Bucket
		urn1 := regexp.MustCompile(`\/[^:]+`).ReplaceAllString(urn0, "")
		// convert sst:sst:Nextjs to sst:Nextjs
		urn2 := regexp.MustCompile(`sst:sst:`).ReplaceAllString(urn1, "sst:")
		// convert pulumi-nodejs:dynamic:Resource to sst:xxxx
		urn3 := urn2
		resourceName1 := resourceName0
		resourceType := regexp.MustCompile(`\.sst\.(.+)$`).FindStringSubmatch(resourceName0)
		if regexp.MustCompile(`pulumi-nodejs:dynamic:Resource$`).MatchString(urn2) &&
			len(resourceType) > 1 {
			urn3 = regexp.MustCompile(`pulumi-nodejs:dynamic:Resource$`).ReplaceAllString(urn2, resourceType[1])
			resourceName1 = regexp.MustCompile(`\.sst\..+$`).ReplaceAllString(resourceName0, "")
		}
		urn4 := regexp.MustCompile(`\$`).ReplaceAllString(urn3, " → ")
		// convert Nextjs$aws:s3:Bucket to Nextjs → aws:s3:Bucket
		urn5 := regexp.MustCompile(`\$`).ReplaceAllString(urn4, " → ")
		return urn5 + " → " + resourceName1
	}

	dedupe := map[string]bool{}
	printProgress := func(progress Progress) {
		spin.Disable()
		dedupeKey := progress.URN + progress.Label
		if dedupe[dedupeKey] {
			return
		}
		dedupe[dedupeKey] = true
		defer spin.Enable()
		if !progress.Final && false {
			pending[progress.URN] =
				color.New(color.FgWhite).Sprintf("   %-11s %v", progress.Label, formatURN(progress.URN))
			suffix := "  Deploying...\n"
			for _, item := range pending {
				suffix += item + "\n"
			}
			spin.Suffix = strings.TrimRight(suffix, "\n")
			return
		}

		color.New(progress.Color, color.Bold).Print("|  ")
		color.New(color.FgHiBlack).Print(fmt.Sprintf("%-11s", progress.Label), " ", formatURN(progress.URN))
		if progress.Duration != 0 {
			color.New(color.FgHiBlack).Printf(" (%s)", progress.Duration)
		}
		if progress.Message != "" {
			color.New(color.FgHiBlack).Print(" ", progress.Message)
		}
		fmt.Println()
	}

	timing := make(map[string]time.Time)
	type errorStatus struct {
		Error string
		URN   string
	}
	errors := []errorStatus{}
	outputs := make(map[string]interface{})

	for evt := range events {
		if evt.SummaryEvent != nil {
			spin.Suffix = "  Finalizing..."
		}
		if evt.ConcurrentUpdateEvent != nil {
			spin.Disable()
			fmt.Println("Concurrent update detected, run `sst cancel` to delete lock file and retry.")
			return false
		}

		if evt.StdOutEvent != nil {
			spin.Disable()
			fmt.Println(evt.StdOutEvent.Text)
			spin.Enable()
			continue
		}

		if evt.ResourcePreEvent != nil {
			timing[evt.ResourcePreEvent.Metadata.URN] = time.Now()
			if evt.ResourcePreEvent.Metadata.Type == "pulumi:pulumi:Stack" {
				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpSame {
				printProgress(Progress{
					Color: color.FgHiBlack,
					Label: "Skipped",
					Final: true,
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})
				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpCreate {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Creating",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})
				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpUpdate {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Updating",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})
				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpCreateReplacement {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Creating",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})

				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpDeleteReplaced {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Deleting",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})

				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpReplace {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Creating",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})

				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpDelete {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Deleting",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})
				continue
			}

			if evt.ResourcePreEvent.Metadata.Op == apitype.OpRefresh {
				printProgress(Progress{
					Color: color.FgYellow,
					Label: "Refreshing",
					URN:   evt.ResourcePreEvent.Metadata.URN,
				})
				continue
			}
		}

		if evt.ResOutputsEvent != nil {
			if evt.ResOutputsEvent.Metadata.Type == "pulumi:pulumi:Stack" && evt.ResOutputsEvent.Metadata.Op != apitype.OpDelete {
				outputs = evt.ResOutputsEvent.Metadata.New.Outputs
				continue
			}
			duration := time.Since(timing[evt.ResOutputsEvent.Metadata.URN]).Round(time.Millisecond)
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpSame && mode == ProgressModeRefresh {
				printProgress(Progress{
					Color:    color.FgGreen,
					Label:    "Refreshed",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
				continue
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpCreate {
				printProgress(Progress{
					Color:    color.FgGreen,
					Label:    "Created",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpUpdate {
				printProgress(Progress{
					Color:    color.FgGreen,
					Label:    "Updated",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpDelete {
				printProgress(Progress{
					Color:    color.FgRed,
					Label:    "Deleted",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpDeleteReplaced {
				printProgress(Progress{
					Color:    color.FgRed,
					Label:    "Deleted",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpCreateReplacement {
				printProgress(Progress{
					Color:    color.FgGreen,
					Label:    "Created",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
			if evt.ResOutputsEvent.Metadata.Op == apitype.OpReplace {
				printProgress(Progress{
					Color:    color.FgGreen,
					Label:    "Created",
					Final:    true,
					URN:      evt.ResOutputsEvent.Metadata.URN,
					Duration: duration,
				})
			}
		}

		if evt.ResOpFailedEvent != nil {
		}

		if evt.DiagnosticEvent != nil {
			if evt.DiagnosticEvent.Severity == "error" {
				if evt.DiagnosticEvent.URN != "" {
					msg := evt.DiagnosticEvent.Message
					lines := strings.Split(evt.DiagnosticEvent.Message, "\n")
					if len(lines) > 2 {
						lines = strings.Split(lines[1], ":")
						msg = strings.TrimSpace(lines[len(lines)-1])
					}
					errors = append(errors, errorStatus{
						Error: msg,
						URN:   evt.DiagnosticEvent.URN,
					})
					printProgress(Progress{
						URN:     evt.DiagnosticEvent.URN,
						Color:   color.FgRed,
						Final:   true,
						Label:   "Error",
						Message: msg,
					})
					continue
				}

				lines := strings.Split(evt.DiagnosticEvent.Message, "\n")
				out := []string{}
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if trimmed == "" {
						continue
					}
					if strings.HasPrefix(trimmed, "at") {
						trimmed = "   " + trimmed
					}
					out = append(out, trimmed)
				}
				if len(out) > 1 {
					errors = append(errors, errorStatus{
						Error: strings.Join(out, "\n"),
					})
				}
			}
		}
	}

	spin.Stop()

	if len(errors) == 0 {
		color.New(color.FgGreen, color.Bold).Print("\n✔")

		if len(outputs) > 0 {
			color.New(color.FgWhite, color.Bold).Println("  Complete:")
			for k, v := range outputs {
				color.New(color.FgHiBlack).Print("   ")
				color.New(color.FgHiBlack, color.Bold).Print(k + ": ")
				color.New(color.FgWhite).Println(v)
			}
		} else {
			color.New(color.FgWhite, color.Bold).Println("  Complete")
		}
		return true
	} else {
		color.New(color.FgRed, color.Bold).Print("\n❌")
		color.New(color.FgWhite, color.Bold).Println(" Failed:")

		for _, status := range errors {
			color.New(color.FgHiBlack).Print("   ")
			if status.URN != "" {
				color.New(color.FgRed, color.Bold).Print(formatURN(status.URN) + ": ")
			}
			color.New(color.FgWhite).Println(strings.TrimSpace(status.Error))
		}
		return false
	}
}
