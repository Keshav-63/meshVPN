package domain

type ResourcePackage string

const (
	PackageSmall  ResourcePackage = "small"
	PackageMedium ResourcePackage = "medium"
	PackageLarge  ResourcePackage = "large"
)

type PackageSpec struct {
	CPUCores    float64
	MemoryMB    int
	MaxReplicas int
}

// PackageSpecs defines the resource allocation for each package tier
var PackageSpecs = map[ResourcePackage]PackageSpec{
	PackageSmall: {
		CPUCores:    0.5,  // 500 millicores
		MemoryMB:    512,  // 512 MB
		MaxReplicas: 3,    // Max 3 replicas for small package
	},
	PackageMedium: {
		CPUCores:    1.0,  // 1000 millicores
		MemoryMB:    1024, // 1 GB
		MaxReplicas: 5,    // Max 5 replicas for medium package
	},
	PackageLarge: {
		CPUCores:    2.0,  // 2000 millicores
		MemoryMB:    2048, // 2 GB
		MaxReplicas: 10,   // Max 10 replicas for large package
	},
}

// GetPackageSpec returns the resource specification for a given package
func GetPackageSpec(pkg ResourcePackage) (PackageSpec, bool) {
	spec, exists := PackageSpecs[pkg]
	return spec, exists
}

// IsValidPackage checks if the package name is valid
func IsValidPackage(pkg string) bool {
	_, exists := PackageSpecs[ResourcePackage(pkg)]
	return exists
}
