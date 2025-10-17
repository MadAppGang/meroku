# Pricing Maintenance Guide

## Overview

This infrastructure uses **curated AWS pricing** that is updated quarterly to ensure cost estimates remain accurate while avoiding the complexity of real-time AWS Pricing API integration.

**Current Pricing Date:** January 15, 2025
**Next Review Date:** April 15, 2025

---

## Why Quarterly Updates?

AWS pricing changes infrequently:
- **Major updates:** 1-2 times per year (typically around AWS re:Invent)
- **Minor adjustments:** Occasional service-specific changes
- **Regional variations:** Minimal impact for common services

**Benefits of Quarterly Manual Updates:**
- ✅ Simple and reliable
- ✅ No API complexity or rate limits
- ✅ Prices are accurate within 3 months
- ✅ Full control over pricing data
- ✅ No dependency on AWS API availability

---

## Update Schedule

| Quarter | Review Date | AWS Events |
|---------|-------------|------------|
| Q1 | January 15 | New Year pricing updates |
| Q2 | April 15 | Post-re:Invent adjustments |
| Q3 | July 15 | Mid-year review |
| Q4 | October 15 | Pre-re:Invent baseline |

---

## How to Update Pricing

### 1. Check for AWS Pricing Changes

Visit the AWS Pricing pages for each service:

- **EC2/Fargate:** https://aws.amazon.com/fargate/pricing/
- **RDS:** https://aws.amazon.com/rds/aurora/pricing/
- **Aurora Serverless v2:** https://aws.amazon.com/rds/aurora/serverless/
- **S3:** https://aws.amazon.com/s3/pricing/
- **EBS (gp3):** https://aws.amazon.com/ebs/pricing/
- **ALB:** https://aws.amazon.com/elasticloadbalancing/pricing/
- **API Gateway:** https://aws.amazon.com/api-gateway/pricing/
- **CloudWatch:** https://aws.amazon.com/cloudwatch/pricing/
- **Route53:** https://aws.amazon.com/route53/pricing/
- **Cognito:** https://aws.amazon.com/cognito/pricing/
- **SES:** https://aws.amazon.com/ses/pricing/
- **EventBridge:** https://aws.amazon.com/eventbridge/pricing/
- **ECR:** https://aws.amazon.com/ecr/pricing/

### 2. Update Fallback Prices

Edit `app/pricing/aws_client.go` in the `getHardcodedFallbackRates()` function:

```go
func getHardcodedFallbackRates() *PriceRates {
	return &PriceRates{
		Region:      "us-east-1",
		Source:      "fallback",
		PricingDate: "YYYY-MM-DD", // ← Update this date

		// RDS Instance Pricing (hourly rates, Single-AZ)
		RDS: map[string]float64{
			"db.t4g.micro":   0.016, // ← Update prices here
			"db.t4g.small":   0.032,
			// ...
		},

		Aurora: AuroraPricing{
			ACUHourly:      0.12,   // ← Update Aurora pricing
			StorageGBMonth: 0.10,
			IORequestsPerM: 0.20,
		},

		// ... update other services
	}
}
```

### 3. Update the Pricing Date Constant

Also update the constant at the top of the same file:

```go
const FALLBACK_PRICING_DATE = "YYYY-MM-DD" // Update to current date
```

### 4. Update Documentation Comments

Update the documentation comment above `getHardcodedFallbackRates()`:

```go
// Last Updated: January 15, 2025 ← Update this
// Next Review: April 15, 2025     ← Update this
```

### 5. Run Tests

Ensure pricing calculations still work correctly:

```bash
cd app
go test ./pricing/... -v
```

All tests should pass. If any fail, check if the pricing formulas need adjustment.

### 6. Update Frontend Display (Optional)

The frontend automatically displays the pricing date from the API. No changes needed unless you want to update the "Next review" date in the tooltip.

Edit `web/src/components/PricingInfo.tsx` if needed:

```tsx
{rates.source === "fallback" && (
  <p className="text-xs mt-1 text-gray-400">
    Next review: April 2025  {/* ← Update quarter here */}
  </p>
)}
```

### 7. Commit Changes

```bash
git add app/pricing/aws_client.go web/src/components/PricingInfo.tsx
git commit -m "chore: Update AWS pricing to [DATE]

- Updated fallback pricing rates from AWS public pricing
- Source: AWS pricing pages (us-east-1 region)
- Next review: [NEXT_QUARTER_DATE]
"
```

### 8. Deploy

Build and deploy the updated binary:

```bash
make build
# Deploy according to your deployment process
```

---

## Pricing Comparison Tool

To verify your updates, you can compare with AWS Calculator:

1. Go to: https://calculator.aws
2. Configure services matching your defaults
3. Compare monthly estimates with your pricing

Example configuration:
- **Fargate:** 0.25 vCPU, 512 MB, 1 task, 730 hours/month
- **Aurora Serverless v2:** 0-1 ACU range, startup level (75% active time)
- **RDS:** db.t4g.micro, 20GB gp3, single-AZ

---

## Future: Real-Time API Integration

If quarterly updates become too cumbersome, consider implementing real-time AWS Pricing API integration:

**Estimated Implementation:** 20-30 hours
**Files to modify:** `app/pricing/aws_client.go:fetchRatesOnce()`

**Benefits:**
- Always current pricing
- Region-specific accuracy
- Automatic updates

**Drawbacks:**
- API complexity (different structure per service)
- Potential rate limits
- Added maintenance burden
- Network dependency

**Recommendation:** Only implement if pricing updates occur more than monthly OR if regional price differences become significant.

---

## Troubleshooting

### Prices seem off after update

1. Check the AWS region (should be us-east-1)
2. Verify you're looking at On-Demand pricing (not Reserved/Spot)
3. Confirm Single-AZ pricing for RDS (Multi-AZ is 2x)
4. For Fargate, ensure you're using per-hour rates (not per-second)

### Tests failing after update

1. Check `app/pricing/calculators_test.go` for expected values
2. Update test expectations if prices changed significantly
3. Verify calculation formulas are still correct

### Frontend not showing new date

1. Clear browser cache and session storage
2. Rebuild frontend: `cd web && npm run build`
3. Verify API endpoint returns new date: `curl http://localhost:8080/api/pricing/rates | jq .pricingDate`

---

## Quick Reference

| Service | Key Metric | Typical Price (us-east-1) |
|---------|------------|---------------------------|
| **Fargate vCPU** | Per vCPU/hour | ~$0.04048 |
| **Fargate Memory** | Per GB/hour | ~$0.004445 |
| **Aurora ACU** | Per ACU/hour | ~$0.12 |
| **RDS t4g.micro** | Per hour | ~$0.016 |
| **EBS gp3** | Per GB/month | ~$0.115 |
| **S3 Standard** | Per GB/month | ~$0.023 |
| **ALB** | Per hour | ~$0.0225 |
| **API Gateway REST** | Per million requests | ~$3.50 |

---

## Contact

For questions about pricing updates:
- Review AWS Pricing documentation
- Check AWS pricing change announcements
- Compare with AWS Pricing Calculator

---

**Last Updated:** January 17, 2025
**Maintainer:** Infrastructure Team
