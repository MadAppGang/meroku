package pricing

// Dated fallback tables for the EC2 compute feature.
//
// This file is the "no credentials, no network, no permission" answer: every
// compute endpoint returns HTTP 200 with the best payload it has, and when
// nothing better exists that payload comes from here. It deliberately imports
// nothing -- no EC2 SDK, no pricing SDK -- so it can never be the thing that
// fails.
//
// Two dates, because two different things go stale at different rates:
//
//   - FALLBACK_INSTANCE_DATA_DATE dates the hardware records (vCPU, memory,
//     architecture, ENI limits). These change only when AWS ships a family.
//   - FALLBACK_PRICING_DATE (aws_client.go) dates the money. It is shared with
//     the rest of the fallback price table so that one envelope field,
//     pricingDate, describes every price in a degraded response.
//
// And one region, which is the trap this file exists to label: the prices below
// are us-east-1 list prices. An envelope built from them is labelled with the
// ENVIRONMENT's region, so it must also carry pricingRegion =
// FALLBACK_PRICING_REGION and let the UI say "indicative us-east-1 pricing"
// rather than print a currency figure attributed to the selected region.
// ap-southeast-2 runs 15-25% above us-east-1 and the premium is not uniform
// across families, so both the displayed cost and the ranking are affected.

// FALLBACK_INSTANCE_DATA_DATE tracks when the instance-type records below were
// captured from the public AWS instance-type catalogue. Update it whenever a
// record is added or corrected. It surfaces as the catalog envelope's
// instanceDataDate field whenever these records are served.
const FALLBACK_INSTANCE_DATA_DATE = "2026-08-18"

// FALLBACK_PRICING_REGION names the region the fallback prices actually
// describe. It surfaces as the catalog envelope's pricingRegion field whenever
// any price came from this table, and is null in the envelope when every price
// came from the Pricing API for the requested region.
const FALLBACK_PRICING_REGION = "us-east-1"

// FALLBACK_AVAILABILITY_VERIFIED is the value the catalog envelope's
// availabilityVerified field MUST take whenever anything from this file is
// served. It is false and it is a constant, not a variable: this table is
// region-agnostic, so nothing here has been checked against the region the user
// selected. The UI must never let a fallback record imply that an instance type
// exists in a region nobody asked AWS about.
const FALLBACK_AVAILABILITY_VERIFIED = false

// FallbackInstanceType is one hardware record, projected to exactly the fields
// the catalog response needs. It mirrors the live DescribeInstanceTypes
// projection so that a degraded response has the same shape as a live one.
//
// Nullable fields are pointers because the API contract distinguishes "zero"
// from "unknown": a GPU count of 0 is a fact, a GPU memory of null is an
// absence.
type FallbackInstanceType struct {
	InstanceType             string   `json:"instanceType"`
	VCPU                     int      `json:"vcpu"`
	MemoryMiB                int      `json:"memoryMiB"`
	Architectures            []string `json:"architectures"`
	CurrentGeneration        bool     `json:"currentGeneration"`
	NetworkPerformance       string   `json:"networkPerformance"`
	BaselineBandwidthMbps    *float64 `json:"baselineBandwidthMbps"`
	MaximumNetworkInterfaces int      `json:"maximumNetworkInterfaces"`
	GPUCount                 int      `json:"gpuCount"`
	GPUMemoryMiB             *int     `json:"gpuMemoryMiB"`
	GPUName                  *string  `json:"gpuName"`
	Burstable                bool     `json:"burstable"`
	BareMetal                bool     `json:"bareMetal"`
	SupportedUsageClasses    []string `json:"supportedUsageClasses"`
}

// FallbackCatalog is the whole degraded-mode catalogue: the records plus the
// three markers that must travel with them. They are returned together, in one
// value, so that a caller cannot pick up the data and forget the caveats.
type FallbackCatalog struct {
	InstanceTypes []FallbackInstanceType

	// InstanceDataDate is FALLBACK_INSTANCE_DATA_DATE.
	InstanceDataDate string
	// PricingDate is FALLBACK_PRICING_DATE -- the money is dated separately
	// from the hardware.
	PricingDate string
	// PricingRegion is FALLBACK_PRICING_REGION. Non-empty means "these prices
	// are not the requested region's" (C-18).
	PricingRegion string
	// AvailabilityVerified is FALLBACK_AVAILABILITY_VERIFIED, i.e. always
	// false. No region was consulted, so no availability claim is made.
	AvailabilityVerified bool
}

// GetFallbackCatalog returns a deep copy of the fallback catalogue, so a caller
// that sorts or filters it in place cannot corrupt the table for the next
// request.
func GetFallbackCatalog() FallbackCatalog {
	src := fallbackInstanceTypes()
	out := make([]FallbackInstanceType, len(src))
	for i, r := range src {
		out[i] = r
		out[i].Architectures = append([]string(nil), r.Architectures...)
		out[i].SupportedUsageClasses = append([]string(nil), r.SupportedUsageClasses...)
		if r.BaselineBandwidthMbps != nil {
			v := *r.BaselineBandwidthMbps
			out[i].BaselineBandwidthMbps = &v
		}
		if r.GPUMemoryMiB != nil {
			v := *r.GPUMemoryMiB
			out[i].GPUMemoryMiB = &v
		}
		if r.GPUName != nil {
			v := *r.GPUName
			out[i].GPUName = &v
		}
	}

	return FallbackCatalog{
		InstanceTypes:        out,
		InstanceDataDate:     FALLBACK_INSTANCE_DATA_DATE,
		PricingDate:          FALLBACK_PRICING_DATE,
		PricingRegion:        FALLBACK_PRICING_REGION,
		AvailabilityVerified: FALLBACK_AVAILABILITY_VERIFIED,
	}
}

// GetFallbackEC2Hourly returns a copy of the on-demand hourly price map,
// instance type -> USD/hour, Linux, shared tenancy, us-east-1 list price.
//
// Every value is strictly greater than zero. A zero here would make the
// recommender's minimum cost zero and every cost sub-score collapse, so "no
// price" is expressed by the type being absent from the map, never by a 0.
func GetFallbackEC2Hourly() map[string]float64 {
	src := fallbackEC2Hourly()
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// FallbackEC2HourlyFor returns the fallback on-demand hourly price for one
// instance type. The bool is false when the type is not in the table -- which
// the caller must render as "price unknown", never as free.
func FallbackEC2HourlyFor(instanceType string) (float64, bool) {
	v, ok := fallbackEC2Hourly()[instanceType]
	return v, ok
}

// FallbackDefaultPoolInstanceTypes returns the instance types the generator
// synthesizes into a zero-config pool. The fallback catalogue is required to
// cover all three, so that a user with no credentials still sees the pool they
// are about to create. It is a function, not a package var, so that a caller
// cannot reorder or truncate the list for everyone else.
func FallbackDefaultPoolInstanceTypes() []string {
	return []string{"m7i-flex.large", "m6i.large", "m6a.large"}
}

// fallbackInstanceTypes is the table itself: the three default-pool types, plus
// one representative of each of the t, c, r and g families in BOTH
// architectures, plus one arm64 general-purpose type so that a recommendation
// made under ami_family al2023_arm64 has a general-purpose candidate to
// substitute when the default pool's x86 types are unavailable.
//
// Values are the public catalogue figures for each type as of
// FALLBACK_INSTANCE_DATA_DATE. maximumNetworkInterfaces is the raw un-trunked
// ENI limit exactly as DescribeInstanceTypes reports it -- it is not a task
// density, and under bridge networking it does not bound density at all.
func fallbackInstanceTypes() []FallbackInstanceType {
	return []FallbackInstanceType{
		// --- general purpose, x86_64: the synthesized default pool ---
		{
			InstanceType:             "m7i-flex.large",
			VCPU:                     2,
			MemoryMiB:                8192,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 3,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "m6i.large",
			VCPU:                     2,
			MemoryMiB:                8192,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 3,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "m6a.large",
			VCPU:                     2,
			MemoryMiB:                8192,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 3,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		// --- general purpose, arm64 ---
		{
			InstanceType:             "m7g.large",
			VCPU:                     2,
			MemoryMiB:                8192,
			Architectures:            []string{"arm64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 4,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		// --- burstable ---
		{
			InstanceType:             "t3.medium",
			VCPU:                     2,
			MemoryMiB:                4096,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(512),
			MaximumNetworkInterfaces: 3,
			Burstable:                true,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "t4g.medium",
			VCPU:                     2,
			MemoryMiB:                4096,
			Architectures:            []string{"arm64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(512),
			MaximumNetworkInterfaces: 3,
			Burstable:                true,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		// --- compute optimized ---
		{
			InstanceType:             "c7i.large",
			VCPU:                     2,
			MemoryMiB:                4096,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 3,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "c7g.large",
			VCPU:                     2,
			MemoryMiB:                4096,
			Architectures:            []string{"arm64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 4,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		// --- memory optimized ---
		{
			InstanceType:             "r7i.large",
			VCPU:                     2,
			MemoryMiB:                16384,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 3,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "r7g.large",
			VCPU:                     2,
			MemoryMiB:                16384,
			Architectures:            []string{"arm64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 12.5 Gigabit",
			BaselineBandwidthMbps:    fbFloat(781),
			MaximumNetworkInterfaces: 4,
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		// --- GPU ---
		{
			InstanceType:             "g5.xlarge",
			VCPU:                     4,
			MemoryMiB:                16384,
			Architectures:            []string{"x86_64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 10 Gigabit",
			BaselineBandwidthMbps:    fbFloat(2500),
			MaximumNetworkInterfaces: 4,
			GPUCount:                 1,
			GPUMemoryMiB:             fbInt(24576),
			GPUName:                  fbString("A10G"),
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
		{
			InstanceType:             "g5g.xlarge",
			VCPU:                     4,
			MemoryMiB:                8192,
			Architectures:            []string{"arm64"},
			CurrentGeneration:        true,
			NetworkPerformance:       "Up to 10 Gigabit",
			BaselineBandwidthMbps:    fbFloat(1250),
			MaximumNetworkInterfaces: 4,
			GPUCount:                 1,
			GPUMemoryMiB:             fbInt(16384),
			GPUName:                  fbString("T4G"),
			SupportedUsageClasses:    []string{"on-demand", "spot"},
		},
	}
}

// fallbackEC2Hourly is the on-demand price side of the same table: USD/hour,
// Linux, shared tenancy, no pre-installed software, us-east-1 list price as of
// FALLBACK_PRICING_DATE. Every instance type in fallbackInstanceTypes has an
// entry, and every entry is > 0.
func fallbackEC2Hourly() map[string]float64 {
	return map[string]float64{
		// general purpose, x86_64 (the synthesized default pool)
		"m7i-flex.large": 0.09576,
		"m6i.large":      0.096,
		"m6a.large":      0.0864,
		// general purpose, arm64
		"m7g.large": 0.0816,
		// burstable
		"t3.medium":  0.0416,
		"t4g.medium": 0.0336,
		// compute optimized
		"c7i.large": 0.08925,
		"c7g.large": 0.0725,
		// memory optimized
		"r7i.large": 0.1323,
		"r7g.large": 0.1071,
		// GPU
		"g5.xlarge":  1.006,
		"g5g.xlarge": 0.42,
	}
}

// fbFloat, fbInt and fbString take the address of a literal, for the nullable
// record fields.
func fbFloat(v float64) *float64 { return &v }
func fbInt(v int) *int           { return &v }
func fbString(v string) *string  { return &v }
