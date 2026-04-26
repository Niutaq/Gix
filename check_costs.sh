#!/bin/bash
# Gix Project - FinOps Cost Analysis Script

echo "--- FinOps Cost Analysis (FOCUS Standard) ---"

# 1. Check if Infracost is installed
if ! command -v infracost &> /dev/null; then
    echo "ERROR: Infracost CLI is not installed. Please install it from https://infracost.io" >&2
    exit 1
fi
fi

# 2. Generate cost report (breakdown)
echo "Analyzing Terraform code..."
PROJECT_DIR="/Users/niutaq/GolandProjects/Gix"
infracost breakdown --path "$PROJECT_DIR/terraform/" \
                   --format json \
                   --out-file "$PROJECT_DIR/infracost_report.json"

# 3. Display table in console
infracost breakdown --path "$PROJECT_DIR/terraform/"

# 4. Extract key metrics (FinOps focus)
# Note: requires 'jq' to be installed
if command -v jq &> /dev/null
then
    TOTAL_COST=$(jq -r '.totalMonthlyCost // 0' "$PROJECT_DIR/infracost_report.json")
    if [ "$TOTAL_COST" == "null" ] || [ -z "$TOTAL_COST" ]; then
        TOTAL_COST=$(jq -r '.projects[0].breakdown.totalMonthlyCost // 0' "$PROJECT_DIR/infracost_report.json")
    fi
    echo "----------------------------------------------"
    echo "ESTIMATED MONTHLY COST (AWS): $TOTAL_COST USD"
    echo "----------------------------------------------"

    # FINOPS GUARDRAIL: Limit for dev environment
    LIMIT=150
    if (( $(echo "$TOTAL_COST > $LIMIT" | bc -l) )); then
        echo "❌ FINOPS ALERT: Estimated cost ($TOTAL_COST) exceeds the limit ($LIMIT USD)!"
        echo "Please optimize your resources (use SPOT, smaller instances, etc.)."
    else
        echo "✅ FINOPS OK: Estimated cost is within the budget ($LIMIT USD)."
    fi
else
    echo "Note: Install 'jq' to see the summary total cost here."
fi
i
