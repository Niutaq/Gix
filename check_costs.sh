#!/usr/bin/env bash
# Gix — optional Infracost guardrail for terraform/ (AWS reference, not default runtime).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "--- FinOps Cost Analysis (Infracost / FOCUS reference) ---"
echo "Note: day-to-day FinOps is local OpenCost (task finops:bootstrap). This script models terraform/ only."

if ! command -v infracost &> /dev/null; then
    echo "ERROR: Infracost CLI is not installed. Please install it from https://infracost.io" >&2
    exit 1
fi

REPORT="${ROOT}/infracost_report.json"

echo "Analyzing Terraform code in ${ROOT}/terraform ..."
infracost breakdown --path "${ROOT}/terraform/" \
                   --format json \
                   --out-file "${REPORT}"

infracost breakdown --path "${ROOT}/terraform/"

if command -v jq &> /dev/null; then
    TOTAL_COST=$(jq -r '.totalMonthlyCost // 0' "${REPORT}")
    if [ "${TOTAL_COST}" == "null" ] || [ -z "${TOTAL_COST}" ]; then
        TOTAL_COST=$(jq -r '.projects[0].breakdown.totalMonthlyCost // 0' "${REPORT}")
    fi
    echo "----------------------------------------------"
    echo "ESTIMATED MONTHLY COST (AWS reference): ${TOTAL_COST} USD"
    echo "----------------------------------------------"

    LIMIT=150
    if (( $(echo "${TOTAL_COST} > ${LIMIT}" | bc -l) )); then
        echo "FINOPS ALERT: Estimated cost (${TOTAL_COST}) exceeds the limit (${LIMIT} USD)!"
        echo "Please optimize resources (SPOT, smaller instances, etc.) or stay on the local FOSS path."
    else
        echo "FINOPS OK: Estimated cost is within the budget (${LIMIT} USD)."
    fi
else
    echo "Note: Install 'jq' to see the summary total cost here."
fi
