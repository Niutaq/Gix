# FinOps Strategy for Gix Project

This document describes the implementation of the **FOCUS (FinOps Open Cost & Usage Specification)** standard and Cloud Ops practices for cloud cost optimization.

## 1. Tagging Standard (FOCUS User Tags)

All AWS resources must have the following tags defined in Terraform (`provider.tf`):

| Tag | Description | FOCUS Mapping |
| :--- | :--- | :--- |
| `Project` | Project name (Gix) | `x_Project` |
| `Environment` | dev, staging, prod | `x_Environment` |
| `focus:service_category` | Service category (compute, database, network) | `ServiceCategory` |
| `ManagedBy` | Infrastructure as Code tool | `x_AutomationTool` |

## 2. Cost-Effective Architecture Design

### Compute (EKS)
- **Challenge:** EKS Control Plane costs ~$72/month.
- **Solution (FinOps):** Use **Fargate** for small workloads (pay-per-usage) or **Spot Instances** for worker nodes managed by **Karpenter** (up to 90% savings).

### Database (RDS)
- **Selection:** `t4g` instances (AWS Graviton2).
- **Reason:** 20% cheaper than `t3` with similar or better performance.
- **Storage:** `gp3` – allows independent configuration of IOPS and throughput (cheaper than `gp2`).

## 3. FOCUS Implementation

Data from **AWS CUR (Cost and Usage Report)** is mapped to the FOCUS 1.0 schema:

1. **ChargePeriod:** Monthly billing cycle.
2. **ProviderName:** `AWS`.
3. **AvailabilityZone:** Extracted directly from AWS metadata.
4. **ServiceCategory:** Mapped based on the `focus:service_category` tag or AWS service type.

## 4. Monitoring and Alerts

- **AWS Budgets:** Set a $10/month limit for the `dev` environment.
- **Anomaly Detection:** Slack/Email notifications for unexpected cost spikes.

## 5. Cost Comparison (Monthly Estimates)

| Service | AWS (Estimated) | DigitalOcean (Actual) | Notes |
| :--- | :--- | :--- | :--- |
| **Cluster (K8s)** | $73.00 (EKS) | ~$12.00 (DOKS/Droplets) | AWS has a fixed control plane fee. |
| **Compute (Nodes)** | $35.04 (On-demand) | Included | AWS cost can be reduced by 70-90% using Spot. |
| **Database** | $16.61 (RDS + Storage) | ~$15.00 (Managed DB) | RDS provides better scaling/backups. |
| **Other (KMS, Logs)** | ~$3.10 | ~$0.00 | AWS charges for KMS keys and log ingestion. |
| **TOTAL** | **$127.75** | **~$27.00** | **AWS is ~4.7x more expensive for this scale.** |

### FinOps Conclusion
For the current scale of the Gix project, **DigitalOcean is significantly more cost-effective**. Migration to AWS is recommended only when:
1. Traffic requires advanced scaling (Karpenter/Fargate).
2. Project requires specific AWS services (e.g., Bedrock for AI, Managed Kafka).
3. AWS credits are available to offset the fixed EKS costs.
