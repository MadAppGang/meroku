package main

type workloadViewModel struct {
	detailViewModel
	workload Workload
}

func newWorkloadViewModel(w Workload) workloadViewModel {
	return workloadViewModel{
		detailViewModel: detailViewModel{
			title:       "Main settings",
			description: "Configure the workload settings",
		},
		workload: w,
	}
}
