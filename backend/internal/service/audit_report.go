package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
)

type auditReport struct {
	ReportID    string                `json:"reportId"`
	Scope       string                `json:"scope"`
	GeneratedAt string                `json:"generatedAt"`
	GeneratedBy string                `json:"generatedBy"`
	Format      string                `json:"format"`
	DID         string                `json:"did,omitempty"`
	ContentHash string                `json:"contentHash,omitempty"`
	Summary     auditReportSummary    `json:"summary"`
	Resources   []auditReportResource `json:"resources"`
	Events      []auditReportEvent    `json:"events"`
	Findings    []auditReportFinding  `json:"findings"`
}

type auditReportSummary struct {
	TotalEvents int `json:"totalEvents"`
	TotalChecks int `json:"totalChecks"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Warnings    int `json:"warnings"`
	NeedsReview int `json:"needsReview"`
}

type auditReportResource struct {
	DID          string `json:"did"`
	Component    string `json:"component"`
	EventCount   int    `json:"eventCount"`
	FindingCount int    `json:"findingCount"`
}

type auditReportEvent struct {
	Timestamp string         `json:"timestamp"`
	Actor     string         `json:"actor,omitempty"`
	Component string         `json:"component"`
	EventType string         `json:"eventType"`
	DID       string         `json:"did,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type auditReportFinding struct {
	Timestamp      string `json:"timestamp"`
	Component      string `json:"component"`
	EventType      string `json:"eventType"`
	DID            string `json:"did,omitempty"`
	RuleID         string `json:"ruleId,omitempty"`
	Title          string `json:"title,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Message        string `json:"message,omitempty"`
	Requirement    string `json:"requirement,omitempty"`
	ActualValue    any    `json:"actualValue,omitempty"`
	ExpectedValue  any    `json:"expectedValue,omitempty"`
	ExpectedValues []any  `json:"expectedValues,omitempty"`
	Operator       string `json:"operator,omitempty"`
	Path           string `json:"path,omitempty"`
	FieldIri       string `json:"fieldIri,omitempty"`
	OntologyTerm   string `json:"ontologyTerm,omitempty"`
	Actor          string `json:"actor,omitempty"`
}

type auditReportDownload struct {
	ReportID    string             `json:"reportId"`
	Scope       string             `json:"scope"`
	Format      string             `json:"format"`
	ContentType string             `json:"contentType"`
	Filename    string             `json:"filename"`
	Encoding    string             `json:"encoding"`
	Content     string             `json:"content"`
	ContentHash string             `json:"contentHash"`
	Summary     auditReportSummary `json:"summary"`
}

func buildAuditReport(scope, did, generatedBy string, generatedAt time.Time, responses []*processauditandcompliance.PACAuditResponse) auditReport {
	report := auditReport{
		Scope:       scope,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		GeneratedBy: generatedBy,
		Format:      "json",
		DID:         strings.TrimSpace(did),
		Resources:   []auditReportResource{},
		Events:      []auditReportEvent{},
		Findings:    []auditReportFinding{},
	}

	resourceIndex := map[string]int{}
	for _, response := range responses {
		if response == nil {
			continue
		}
		if report.DID != "" && response.Did != report.DID {
			continue
		}
		resourceKey := response.Component + "\x00" + response.Did
		if _, ok := resourceIndex[resourceKey]; !ok {
			resourceIndex[resourceKey] = len(report.Resources)
			report.Resources = append(report.Resources, auditReportResource{
				DID:       response.Did,
				Component: response.Component,
			})
		}
		resource := &report.Resources[resourceIndex[resourceKey]]
		for _, entry := range response.AuditTrail {
			if entry == nil {
				continue
			}
			eventData := objectMap(entry.EventData)
			entryDID := response.Did
			if entry.Did != nil && strings.TrimSpace(*entry.Did) != "" {
				entryDID = strings.TrimSpace(*entry.Did)
			}
			if report.DID != "" && entryDID != report.DID {
				continue
			}
			if isAuditFindingEvent(entry.EventType, eventData) {
				finding := auditReportFinding{
					Timestamp:      entry.CreatedAt,
					Component:      entry.Component,
					EventType:      entry.EventType,
					DID:            entryDID,
					RuleID:         stringFromMap(eventData, "ruleId", "rule_id"),
					Title:          stringFromMap(eventData, "title"),
					Severity:       stringFromMap(eventData, "severity"),
					Message:        stringFromMap(eventData, "message"),
					Requirement:    stringFromMap(eventData, "requirement"),
					ActualValue:    eventData["actualValue"],
					ExpectedValue:  eventData["expectedValue"],
					ExpectedValues: anySlice(eventData["expectedValues"]),
					Operator:       stringFromMap(eventData, "operator"),
					Path:           stringFromMap(eventData, "path"),
					FieldIri:       stringFromMap(eventData, "fieldIri"),
					OntologyTerm:   stringFromMap(eventData, "ontologyTerm", "ontology_term"),
					Actor:          actorFromEventData(eventData),
				}
				report.Findings = append(report.Findings, finding)
				resource.FindingCount++
				continue
			}
			report.Events = append(report.Events, auditReportEvent{
				Timestamp: entry.CreatedAt,
				Actor:     actorFromEventData(eventData),
				Component: entry.Component,
				EventType: entry.EventType,
				DID:       entryDID,
				Details:   eventData,
			})
			resource.EventCount++
		}
	}

	sort.Slice(report.Events, func(i, j int) bool {
		return report.Events[i].Timestamp < report.Events[j].Timestamp
	})
	sort.Slice(report.Findings, func(i, j int) bool {
		return report.Findings[i].Timestamp < report.Findings[j].Timestamp
	})
	report.Summary = summarizeAuditReport(report)
	report.ReportID = auditReportID(scope, did, generatedBy, generatedAt, report.Summary)
	return report
}

func summarizeAuditReport(report auditReport) auditReportSummary {
	summary := auditReportSummary{
		TotalEvents: len(report.Events),
		TotalChecks: len(report.Findings),
	}
	for _, finding := range report.Findings {
		switch normalizedSeverity(finding.Severity) {
		case "passed":
			summary.Passed++
		case "failed":
			summary.Failed++
		case "warning":
			summary.Warnings++
			summary.NeedsReview++
		default:
			summary.NeedsReview++
		}
	}
	return summary
}

func normalizedSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "info", "ok", "passed", "pass", "success", "successful", "compliant":
		return "passed"
	case "error", "critical", "blocking", "failed", "fail", "violation", "non_compliant":
		return "failed"
	case "warning", "warn":
		return "warning"
	default:
		return "review"
	}
}

func isAuditFindingEvent(eventType string, data map[string]any) bool {
	normalized := strings.ToUpper(strings.TrimSpace(eventType))
	if strings.Contains(normalized, "POLICY_AUDIT_FINDING") || strings.Contains(normalized, "COMPLIANCE_FINDING") {
		return true
	}
	if strings.Contains(normalized, "AUDIT_CHECK") {
		return true
	}
	return stringFromMap(data, "ruleId", "severity", "requirement") != ""
}

func objectMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return objectMapFromBytes(raw)
	}
	if raw, ok := value.([]byte); ok {
		return objectMapFromBytes(raw)
	}
	if obj, ok := value.(map[string]any); ok {
		return obj
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return objectMapFromBytes(bytes)
}

func objectMapFromBytes(bytes []byte) map[string]any {
	var obj map[string]any
	if err := json.Unmarshal(bytes, &obj); err != nil {
		return nil
	}
	return obj
}

func actorFromEventData(data map[string]any) string {
	for _, key := range []string{
		"actor", "user", "username", "generated_by", "generatedBy", "audited_by", "auditedBy",
		"created_by", "createdBy", "approved_by", "approvedBy", "signed_by", "signedBy",
		"signer_did", "signerDid", "stored_by", "storedBy", "submitted_by", "submittedBy",
		"reviewed_by", "reviewedBy", "verified_by", "verifiedBy", "rejected_by", "rejectedBy",
	} {
		if value := stringFromMap(data, key); value != "" {
			return value
		}
	}
	return ""
}

func stringFromMap(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
			if value != nil {
				return strings.TrimSpace(fmt.Sprint(value))
			}
		}
	}
	return ""
}

func anySlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func auditReportID(scope, did, generatedBy string, generatedAt time.Time, summary auditReportSummary) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d", scope, did, generatedBy, generatedAt.UTC().Format(time.RFC3339Nano), summary.TotalEvents, summary.TotalChecks, summary.Failed)
	sum := sha256.Sum256([]byte(payload))
	return "pac-report-" + hex.EncodeToString(sum[:8])
}

func hashBytes(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func renderAuditReportCSV(report auditReport) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	rows := [][]string{
		{"section", "timestamp", "did", "component", "eventType", "actor", "result", "ruleId", "message", "requirement", "actualValue", "expectedValue", "expectedValues", "path"},
	}
	for _, event := range report.Events {
		rows = append(rows, []string{"event", event.Timestamp, event.DID, event.Component, event.EventType, event.Actor, "", "", "", "", "", "", "", ""})
	}
	for _, finding := range report.Findings {
		rows = append(rows, []string{
			"finding",
			finding.Timestamp,
			finding.DID,
			finding.Component,
			finding.EventType,
			finding.Actor,
			normalizedSeverity(finding.Severity),
			finding.RuleID,
			finding.Message,
			finding.Requirement,
			formatReportValue(finding.ActualValue),
			formatReportValue(finding.ExpectedValue),
			formatReportValue(finding.ExpectedValues),
			firstNonEmpty(finding.FieldIri, finding.Path),
		})
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderAuditReportPDF(report auditReport) []byte {
	lines := []string{
		"FACIS Audit Report",
		"Report ID: " + report.ReportID,
		"Scope: " + report.Scope,
		"Generated at: " + report.GeneratedAt,
		"Generated by: " + report.GeneratedBy,
		fmt.Sprintf("Summary: %d events, %d checks, %d passed, %d failed, %d warnings, %d needs review", report.Summary.TotalEvents, report.Summary.TotalChecks, report.Summary.Passed, report.Summary.Failed, report.Summary.Warnings, report.Summary.NeedsReview),
		"",
		"Findings",
	}
	for _, finding := range report.Findings {
		lines = append(lines, wrapPDFLine(fmt.Sprintf("%s [%s] %s %s", finding.Timestamp, finding.Severity, finding.RuleID, finding.Message))...)
		if finding.Requirement != "" {
			lines = append(lines, wrapPDFLine("Requirement: "+finding.Requirement)...)
		}
	}
	if len(report.Findings) == 0 {
		lines = append(lines, "No compliance findings.")
	}
	lines = append(lines, "", "Lifecycle Events")
	for _, event := range report.Events {
		lines = append(lines, wrapPDFLine(fmt.Sprintf("%s actor=%s %s %s", event.Timestamp, event.Actor, event.EventType, event.DID))...)
	}
	if len(report.Events) == 0 {
		lines = append(lines, "No lifecycle events.")
	}
	return simplePDF(lines)
}

func simplePDF(lines []string) []byte {
	var text bytes.Buffer
	text.WriteString("BT\n/F1 10 Tf\n50 780 Td\n14 TL\n")
	for _, line := range lines {
		text.WriteString("(")
		text.WriteString(escapePDFText(line))
		text.WriteString(") Tj\nT*\n")
	}
	text.WriteString("ET\n")
	stream := text.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for i, obj := range objects {
		offsets = append(offsets, out.Len())
		out.WriteString(strconv.Itoa(i + 1))
		out.WriteString(" 0 obj\n")
		out.WriteString(obj)
		out.WriteString("\nendobj\n")
	}
	xref := out.Len()
	out.WriteString("xref\n0 ")
	out.WriteString(strconv.Itoa(len(objects) + 1))
	out.WriteString("\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	out.WriteString("trailer\n<< /Size ")
	out.WriteString(strconv.Itoa(len(objects) + 1))
	out.WriteString(" /Root 1 0 R >>\nstartxref\n")
	out.WriteString(strconv.Itoa(xref))
	out.WriteString("\n%%EOF\n")
	return out.Bytes()
}

func wrapPDFLine(line string) []string {
	const max = 95
	if len(line) <= max {
		return []string{line}
	}
	var lines []string
	for len(line) > max {
		lines = append(lines, line[:max])
		line = line[max:]
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func escapePDFText(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`, "\r", " ", "\n", " ")
	return replacer.Replace(value)
}

func formatReportValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(bytes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func reportDownloadEnvelope(report auditReport, format string, content []byte, contentType string) auditReportDownload {
	filename := fmt.Sprintf("%s.%s", report.ReportID, format)
	encoding := "utf-8"
	body := string(content)
	if format == "pdf" {
		encoding = "base64"
		body = base64.StdEncoding.EncodeToString(content)
	}
	return auditReportDownload{
		ReportID:    report.ReportID,
		Scope:       report.Scope,
		Format:      format,
		ContentType: contentType,
		Filename:    filename,
		Encoding:    encoding,
		Content:     body,
		ContentHash: hashBytes(content),
		Summary:     report.Summary,
	}
}
