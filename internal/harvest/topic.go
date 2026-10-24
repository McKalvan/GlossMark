// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"regexp"
	"strings"
)

// Queue, event and scheduled-job names are interfaces of their own (ADR
// 0025, section 4): a topic named loan.returned or a cron job named
// send-overdue-notices says what the system does as plainly as a route. They
// are topic evidence in the code family, with a kind: queue, event or cron.

var (
	// topicKey: a configuration key or argument whose string value names a
	// queue, a topic or an event.
	topicKey = regexp.MustCompile(`(?i)\b(queue|queue_?name|topic|topic_?name|routing_?key|event|event_?name|event_?type|job_?name|schedule_?name|cron_?name)["']?\s*[:=]\s*["']([A-Za-z][\w.\-:/]*)["']`)
	// topicCall: a call that publishes, emits, consumes or schedules by name.
	topicCall = regexp.MustCompile(`\.(publish|emit|subscribe|consume|send_task|enqueue|dispatch|produce|add_job|schedule)\(\s*["']([A-Za-z][\w.\-:/]*)["']`)
	// tfTopic: an infrastructure resource that is a queue, a topic or a schedule.
	tfTopic   = regexp.MustCompile(`^\s*resource\s+"(aws_sqs_queue|aws_sns_topic|google_pubsub_topic|google_pubsub_subscription|azurerm_servicebus_queue|azurerm_servicebus_topic|aws_cloudwatch_event_rule|aws_scheduler_schedule|google_cloud_scheduler_job|aws_kinesis_stream|confluent_kafka_topic|kafka_topic)"\s+"([^"]+)"`)
	yamlKind  = regexp.MustCompile(`^kind:\s*["']?(CronJob)["']?\s*$`)
	yamlName  = regexp.MustCompile(`^\s{2}name:\s*["']?([A-Za-z][\w.\-]*)["']?\s*$`)
	cronEntry = regexp.MustCompile(`^\s*["']([a-z][\w\-]*)["']\s*:\s*\{.*["']?(?:schedule|task|cron)["']?\s*:`)
)

// topicKinds maps a key, call or resource to the kind of topic it names.
func topicKind(what string) string {
	w := strings.ToLower(what)
	switch {
	case strings.Contains(w, "cron"), strings.Contains(w, "schedul"), strings.Contains(w, "job"), strings.Contains(w, "event_rule"):
		return "cron"
	case strings.Contains(w, "event"), w == "emit", w == "publish", w == "dispatch":
		return "event"
	}
	return "queue"
}

// topics records the topic names a code or configuration file declares.
func (h *harvester) topics(rel string, lines []string, ext string) {
	cronJob := false
	for i, ln := range lines {
		n := i + 1
		switch ext {
		case ".tf", ".hcl":
			if m := tfTopic.FindStringSubmatch(ln); m != nil {
				h.add("topic", m[2], rel, n, "kind:"+topicKind(m[1]), "resource:"+m[1])
			}
			continue
		case ".yaml", ".yml":
			if yamlKind.MatchString(ln) {
				cronJob = true
				continue
			}
			if cronJob {
				if m := yamlName.FindStringSubmatch(ln); m != nil {
					h.add("topic", m[1], rel, n, "kind:cron")
					cronJob = false
					continue
				}
			}
		}
		if m := cronEntry.FindStringSubmatch(ln); m != nil {
			h.add("topic", m[1], rel, n, "kind:cron")
			continue
		}
		if m := topicCall.FindStringSubmatch(ln); m != nil && namedTopic(m[2]) {
			h.add("topic", m[2], rel, n, "kind:"+topicKind(m[1]))
			continue
		}
		if m := topicKey.FindStringSubmatch(ln); m != nil && !strings.Contains(m[2], "://") && namedTopic(m[2]) {
			h.add("topic", m[2], rel, n, "kind:"+topicKind(m[1]))
		}
	}
}

// namedTopic keeps names that read as a topic (loan.returned, LoanReturned,
// overdue-notices) and drops single lower-case words (click, change, data),
// which name a UI event or a variable more often than a domain one.
func namedTopic(v string) bool {
	if strings.ContainsAny(v, ".:_-/") {
		return true
	}
	caps := 0
	for _, r := range v {
		if r >= 'A' && r <= 'Z' {
			caps++
		}
	}
	return caps >= 2
}
