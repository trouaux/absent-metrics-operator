// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Alert Rule", func() {
	logger := zap.New(zap.UseDevMode(true))

	// parseRuleAll invokes parseRule and returns the concatenation of bare and
	// labeled rules. parseRule now splits its output into two slices so callers
	// can route them into separate AbsencePrometheusRule CRs, but the unit
	// tests in this file pre-date the split and assert on the combined ordered
	// stream (bare first, then labeled — matching the old behaviour). The
	// helper hides the split so the existing assertions can stay readable.
	parseRuleAll := func(in monitoringv1.Rule, keepLabel KeepLabel, absentLabel AbsentLabel) ([]monitoringv1.Rule, error) {
		bare, labeled, err := parseRule(logger, in, keepLabel, absentLabel)
		if err != nil {
			return nil, err
		}
		return append(bare, labeled...), nil
	}
	keepLabel := KeepLabel{
		LabelSupportGroup: true,
		LabelTier:         true,
		LabelService:      true,
	}

	DescribeTable("Parsing alert rule expressions",
		func(in monitoringv1.Rule, out []monitoringv1.Rule) {
			expected := out
			actual, err := parseRuleAll(in, keepLabel, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(HaveLen(len(expected)))

			// We only check the alert name, expression, and labels. Annotations are hard-coded and
			// don't need to be checked here in unit tests; they are already checked in e2e tests.
			for i, wanted := range expected {
				got := actual[i]
				Expect(got.Alert).To(Equal(wanted.Alert))
				Expect(got.Expr).To(Equal(wanted.Expr))
				Expect(got.Labels).To(Equal(wanted.Labels))
			}
		},
		Entry("alert rule with label matching in expression and templating in labels",
			monitoringv1.Rule{
				Alert: "OpenstackKeppelPodSchedulingInsufficientMemory",
				Expr:  intstr.FromString(`sum(rate(kube_pod_failed_scheduling_memory_total{namespace="keppel"}[30m])) by (pod_name) > 0`),
				Labels: map[string]string{
					"tier":    `{{ $labels.somelabel }}`,
					"service": "keppel",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentKeppelKubePodFailedSchedulingMemoryTotal",
				Expr:  intstr.FromString(`absent(kube_pod_failed_scheduling_memory_total)`),
				Labels: map[string]string{
					"context":  "absent-metrics",
					"severity": "info",
					"service":  "keppel",
				},
			}},
		),
		Entry("alert rule with multiple usage of the same metric in expression",
			monitoringv1.Rule{
				Alert: "OpenstackKeppelPodOOMExceedingLimits",
				Expr:  intstr.FromString(`keppel_container_memory_usage_percent > 70 and predict_linear(keppel_container_memory_usage_percent[1h], 7*3600) > 100`),
				Labels: map[string]string{
					"tier":    "os",
					"service": "keppel",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentOsKeppelContainerMemoryUsagePercent",
				Expr:  intstr.FromString(`absent(keppel_container_memory_usage_percent)`),
				Labels: map[string]string{
					"context":  "absent-metrics",
					"severity": "info",
					"tier":     "os",
					"service":  "keppel",
				},
			}},
		),
		Entry("alert rule with multiple different metrics in the expression",
			monitoringv1.Rule{
				Alert: "OpenstackSwiftHealthCheck",
				Expr:  intstr.FromString(`avg(swift_recon_task_exit_code) BY (region) > 0.2 or avg(swift_dispersion_task_exit_code) BY (region) > 0.2`),
				Labels: map[string]string{
					"support_group": "not-containers",
					"tier":          "os",
					"service":       "swift",
				},
			},
			[]monitoringv1.Rule{
				{
					Alert: "AbsentNotContainersSwiftDispersionTaskExitCode",
					Expr:  intstr.FromString(`absent(swift_dispersion_task_exit_code)`),
					Labels: map[string]string{
						"context":       "absent-metrics",
						"severity":      "info",
						"support_group": "not-containers",
						"tier":          "os",
						"service":       "swift",
					},
				},
				{
					Alert: "AbsentNotContainersSwiftReconTaskExitCode",
					Expr:  intstr.FromString(`absent(swift_recon_task_exit_code)`),
					Labels: map[string]string{
						"context":       "absent-metrics",
						"severity":      "info",
						"support_group": "not-containers",
						"tier":          "os",
						"service":       "swift",
					},
				},
			},
		),
		Entry("complex expression test case 1",
			monitoringv1.Rule{
				Alert: "OpenstackSwiftUsedSpace",
				Expr:  intstr.FromString(`max(predict_linear(global:swift_cluster_storage_used_percent_average[1w], 60 * 60 * 24 * 30)) > 0.8`),
				Labels: map[string]string{
					"support_group": "not-containers",
					"tier":          "os",
					"service":       "swift",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentNotContainersSwiftGlobalSwiftClusterStorageUsedPercentAverage",
				Expr:  intstr.FromString(`absent(global:swift_cluster_storage_used_percent_average)`),
				Labels: map[string]string{
					"context":       "absent-metrics",
					"severity":      "info",
					"support_group": "not-containers",
					"tier":          "os",
					"service":       "swift",
				},
			}},
		),
		Entry("complex expression test case 2",
			monitoringv1.Rule{
				Alert: "OpenstackLimesHttpErrors",
				Expr:  intstr.FromString(`sum(increase(http_requests_total{kubernetes_namespace="limes",code=~"5.*"}[1h])) by (kubernetes_name) > 0`),
				Labels: map[string]string{
					"support_group": "containers",
					"service":       "limes",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentContainersLimesHttpRequestsTotal",
				Expr:  intstr.FromString(`absent(http_requests_total)`),
				Labels: map[string]string{
					"context":       "absent-metrics",
					"severity":      "info",
					"support_group": "containers",
					"service":       "limes",
				},
			}},
		),
		Entry("alert rule that uses label matching against the internal '__name__' label in expression",
			monitoringv1.Rule{
				Alert: "OpenstackLimesSuspendedScrapes",
				Expr:  intstr.FromString(`sum(increase({__name__=~'limes_suspended_scrapes'}[15m])) BY (os_cluster, service, service_name) > 0`),
				Labels: map[string]string{
					"support_group": "containers",
					"service":       "limes",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentContainersLimesSuspendedScrapes",
				Expr:  intstr.FromString(`absent(limes_suspended_scrapes)`),
				Labels: map[string]string{
					"context":       "absent-metrics",
					"severity":      "info",
					"support_group": "containers",
					"service":       "limes",
				},
			}},
		),
		Entry("alert rule that already uses 'absent' function for the metric used in the expression",
			monitoringv1.Rule{
				Alert: "OpenstackLimesFailedScrapes",
				Expr:  intstr.FromString(`absent(limes_failed_scrapes) or sum(increase(limes_failed_scrapes[5m])) BY (os_cluster, service, service_name) > 0`),
				Labels: map[string]string{
					"support_group": "containers",
					"service":       `{{ $labels.service_name }}`,
				},
			},
			nil, // no absence alert rules should be generated for this alert
		),
		Entry("alert rule that uses 'absent' function but for a different metric in the expression",
			monitoringv1.Rule{
				Alert: "OpenstackLimesUnexpectedServiceRoleAssignments",
				Expr:  intstr.FromString(`absent(openstack_assignments_per_service{service_name="service"}) or max(openstack_assignments_per_role{role_name="resource_service"}) > 1`),
				Labels: map[string]string{
					"support_group": "containers",
					"service":       "limes",
				},
			},
			[]monitoringv1.Rule{{
				Alert: "AbsentContainersLimesOpenstackAssignmentsPerRole",
				Expr:  intstr.FromString(`absent(openstack_assignments_per_role)`),
				Labels: map[string]string{
					"context":       "absent-metrics",
					"severity":      "info",
					"support_group": "containers",
					"service":       "limes",
				},
			}},
		),
		Entry("alert rule with 'no_alert_on_absence' label",
			monitoringv1.Rule{
				Alert: "OpenstackSwiftMismatchedRings",
				Expr:  intstr.FromString(`(swift_cluster_md5_not_matched{kind="ring"} - swift_cluster_md5_errors{kind="ring"}) > 0`),
				Labels: map[string]string{
					"support_group":       "not-containers",
					"tier":                "os",
					"service":             "swift",
					"no_alert_on_absence": "true",
				},
			},
			nil, // absence alerts are not generated for record rules
		),
		Entry("record rule",
			monitoringv1.Rule{
				Record: "predict_linear_global_cluster_storage_used_percent_average",
				Expr:   intstr.FromString(`max(predict_linear(global:swift_cluster_storage_used_percent_average[1w], 60 * 60 * 24 * 30)) > 0.8`),
				Labels: map[string]string{
					"support_group": "not-containers",
					"tier":          "os",
					"service":       "swift",
				},
			},
			nil, // absence alerts are not generated for record rules
		),
	)

	Describe("Parsing alert rule expressions with AbsentLabel", func() {
		baseLabels := map[string]string{
			"context":       "absent-metrics",
			"severity":      "info",
			"support_group": "containers",
			"service":       "k8s",
		}
		ruleLabels := map[string]string{
			"support_group": "containers",
			"service":       "k8s",
		}
		nsPod := AbsentLabel{"namespace": true, "pod": true}

		// Each case feeds ONE source alert rule (labels fixed to ruleLabels)
		// through parseRule and asserts the full ordered output stream: the
		// bare absent(metric) rule first, then any labeled absent(metric{…})
		// rules in Expr-string order. wantExprs lists those expressions; every
		// generated rule carries the same derived baseLabels. The source Alert
		// name is irrelevant to the output (generated names derive from the
		// keep-labels + metric name), so a constant is used throughout.
		DescribeTable("bare + labeled emission",
			func(expr string, absentLabel AbsentLabel, wantAlerts, wantExprs []string) {
				actual, err := parseRuleAll(
					monitoringv1.Rule{Alert: "SourceAlert", Expr: intstr.FromString(expr), Labels: ruleLabels},
					keepLabel, absentLabel)
				Expect(err).ToNot(HaveOccurred())
				Expect(actual).To(HaveLen(len(wantAlerts)))
				for i := range wantAlerts {
					Expect(actual[i].Alert).To(Equal(wantAlerts[i]))
					Expect(actual[i].Expr.String()).To(Equal(wantExprs[i]))
					Expect(actual[i].Labels).To(Equal(baseLabels))
				}
			},
			Entry("nil AbsentLabel → only bare rule (pre-feature behaviour)",
				`kube_pod_status_phase{namespace="production",pod="api-server",phase="Failed"} > 0`,
				nil,
				[]string{"AbsentContainersK8sKubePodStatusPhase"},
				[]string{`absent(kube_pod_status_phase)`},
			),
			Entry("empty AbsentLabel → only bare rule (pre-feature behaviour)",
				`kube_pod_status_phase{namespace="production",pod="api-server",phase="Failed"} > 0`,
				AbsentLabel{},
				[]string{"AbsentContainersK8sKubePodStatusPhase"},
				[]string{`absent(kube_pod_status_phase)`},
			),
			Entry("selector carries both requested labels → bare + one labeled",
				`kube_pod_status_phase{namespace="production",pod="api-server",phase="Failed"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sKubePodStatusPhase", "AbsentLabelsContainersK8sKubePodStatusPhase"},
				[]string{`absent(kube_pod_status_phase)`, `absent(kube_pod_status_phase{namespace="production",pod="api-server"})`},
			),
			Entry("selector carries only one requested label → bare + one labeled",
				`my_metric{namespace="staging"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="staging"})`},
			),
			Entry("selector carries none of the requested labels → only bare",
				`my_metric{env="prod"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric"},
				[]string{`absent(my_metric)`},
			),
			Entry("metric used twice with identical labels → bare + one labeled (deduped)",
				`my_metric{namespace="prod"} > 70 and predict_linear(my_metric{namespace="prod"}[1h], 3600) > 100`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="prod"})`},
			),
			Entry("metric used twice with different label values → bare + two labeled (Expr-sorted)",
				`my_metric{namespace="prod"} > 0 or my_metric{namespace="staging"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="prod"})`, `absent(my_metric{namespace="staging"})`},
			),
			Entry("metric used twice, one occurrence lacks the label → bare + one labeled",
				`my_metric{namespace="prod"} > 0 or my_metric{env="other"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="prod"})`},
			),
			Entry("regex matcher on a requested label is preserved in the labeled rule",
				`my_metric{namespace=~"prod.*"} > 0`,
				nsPod,
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace=~"prod.*"})`},
			),
			Entry(`all matcher types (=, !=, =~, !~) are preserved under "*"`,
				`my_metric{namespace="prod",region!="dev",job=~".*compactor.*",pod!~"canary.*"} > 0`,
				AbsentLabel{"*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{job=~".*compactor.*",namespace="prod",pod!~"canary.*",region!="dev"})`},
			),
			Entry(`"*" collects every non-__name__ label`,
				`my_metric{namespace="prod",pod="api",env="staging"} > 0`,
				AbsentLabel{"*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{env="staging",namespace="prod",pod="api"})`},
			),
			Entry(`prefix wildcard "label_*" matches only the prefix`,
				`my_metric{label_a="x",label_b="y",other="z"} > 0`,
				AbsentLabel{"label_*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{label_a="x",label_b="y"})`},
			),
			Entry(`suffix wildcard "*_id" matches only the suffix`,
				`my_metric{request_id="r1",trace_id="t1",namespace="prod"} > 0`,
				AbsentLabel{"*_id": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{request_id="r1",trace_id="t1"})`},
			),
			Entry(`contains wildcard "*ace*" matches the substring`,
				`my_metric{namespace="prod",trace_id="t1",pod="api"} > 0`,
				AbsentLabel{"*ace*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="prod",trace_id="t1"})`},
			),
			Entry(`"*" never matches the internal __name__ label`,
				`{__name__="my_metric",namespace="prod"} > 0`,
				AbsentLabel{"*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{namespace="prod"})`},
			),
			Entry("mixed exact + wildcard patterns collect the union",
				`my_metric{namespace="prod",label_a="x",other="z"} > 0`,
				AbsentLabel{"namespace": true, "label_*": true},
				[]string{"AbsentContainersK8sMyMetric", "AbsentLabelsContainersK8sMyMetric"},
				[]string{`absent(my_metric)`, `absent(my_metric{label_a="x",namespace="prod"})`},
			),
			Entry("pattern matching no label in the selector → only bare",
				`my_metric{env="prod"} > 0`,
				AbsentLabel{"label_*": true},
				[]string{"AbsentContainersK8sMyMetric"},
				[]string{`absent(my_metric)`},
			),
		)
	})

	// These tests exercise ParseRuleGroups itself (not parseRule) — specifically
	// its cross-rule, cross-group dedup. Two distinct source alert rules in the
	// same PrometheusRule can legitimately reference the same metric (e.g. an
	// alert for "down 5m" and another for "down 15m"); without dedup, both
	// invocations of parseRule emit their own absent(metric) rule and the output
	// AbsencePrometheusRule contains two identical rules. This regression was
	// observed in production for absent(opensearch_cluster_status).
	Describe("Cross-rule dedup", func() {
		It("collapses absent() emitted by two source alerts in the same group", func() {
			groups := []monitoringv1.RuleGroup{
				{
					Name: "opensearch.alerts",
					Rules: []monitoringv1.Rule{
						{
							Alert: "OpensearchDown5m",
							Expr:  intstr.FromString(`opensearch_cluster_status{} != 0`),
							Labels: map[string]string{
								"support_group": "containers",
								"service":       "elk",
							},
						},
						{
							Alert: "OpensearchDown15m",
							Expr:  intstr.FromString(`opensearch_cluster_status{} > 1`),
							Labels: map[string]string{
								"support_group": "containers",
								"service":       "elk",
							},
						},
					},
				},
			}
			bare, labeled, err := ParseRuleGroups(logger, groups, "elk-alerts", keepLabel, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(labeled).To(BeNil())
			Expect(bare).To(HaveLen(1))
			Expect(bare[0].Rules).To(HaveLen(1), "expected a single deduped absent(opensearch_cluster_status) rule")
			Expect(bare[0].Rules[0].Expr.String()).To(Equal(`absent(opensearch_cluster_status)`))
		})

		It("collapses absent() emitted by alerts in different groups of the same PrometheusRule", func() {
			groups := []monitoringv1.RuleGroup{
				{
					Name: "opensearch.alerts",
					Rules: []monitoringv1.Rule{{
						Alert: "OpensearchDown",
						Expr:  intstr.FromString(`opensearch_cluster_status != 0`),
						Labels: map[string]string{
							"support_group": "containers",
							"service":       "elk",
						},
					}},
				},
				{
					Name: "opensearch.recording",
					Rules: []monitoringv1.Rule{{
						Alert: "OpensearchDegraded",
						Expr:  intstr.FromString(`opensearch_cluster_status > 0`),
						Labels: map[string]string{
							"support_group": "containers",
							"service":       "elk",
						},
					}},
				},
			}
			bare, _, err := ParseRuleGroups(logger, groups, "elk-alerts", keepLabel, nil)
			Expect(err).ToNot(HaveOccurred())
			// First group keeps the rule; second group emits nothing once
			// deduped, so it does not produce an output RuleGroup at all.
			Expect(bare).To(HaveLen(1))
			Expect(bare[0].Name).To(Equal("elk-alerts/opensearch.alerts"))
			Expect(bare[0].Rules).To(HaveLen(1))
			Expect(bare[0].Rules[0].Expr.String()).To(Equal(`absent(opensearch_cluster_status)`))
		})

		It("splits bare and labeled rules into separate output streams", func() {
			groups := []monitoringv1.RuleGroup{{
				Name: "opensearch.alerts",
				Rules: []monitoringv1.Rule{{
					Alert: "OpensearchDown",
					Expr:  intstr.FromString(`opensearch_cluster_status{namespace="prod"} != 0`),
					Labels: map[string]string{
						"support_group": "containers",
						"service":       "elk",
					},
				}},
			}}
			bare, labeled, err := ParseRuleGroups(logger, groups, "elk-alerts", keepLabel, AbsentLabel{"namespace": true})
			Expect(err).ToNot(HaveOccurred())
			Expect(bare).To(HaveLen(1))
			Expect(labeled).To(HaveLen(1))
			Expect(bare[0].Rules).To(HaveLen(1))
			Expect(bare[0].Rules[0].Expr.String()).To(Equal(`absent(opensearch_cluster_status)`))
			Expect(labeled[0].Rules).To(HaveLen(1))
			Expect(labeled[0].Rules[0].Expr.String()).To(Equal(`absent(opensearch_cluster_status{namespace="prod"})`))
			// Group names match on both sides so the two CRs reflect the same source group structure.
			Expect(bare[0].Name).To(Equal(labeled[0].Name))
		})
	})
})
