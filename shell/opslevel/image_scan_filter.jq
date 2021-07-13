# We do not want to send sensitive info to OpsLevel (e.g. actual vulns and compliance issues).
# Instead, we send compact summary and the Circle CI Artifact URL to allow quick navigation
# from OpsLevel Web to the raw report.
.results | .[0] | {
    id: .id,
    service: $project,
    image_name: .name,
    url: $url,
    complianceScanPassed: .complianceScanPassed,
    complianceDistribution: .complianceDistribution,
    vulnerabilityDistribution: .vulnerabilityDistribution,
    vulnerabilityScanPassed: .vulnerabilityScanPassed,
}
