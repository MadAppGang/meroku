package main

type workloadViewModel struct {
	detailViewModel
	workload workload
}

func newWorkloadViewModel(w workload) workloadViewModel {
	return workloadViewModel{
		detailViewModel: detailViewModel{
			title:       "Main settings",
			description: "Configure the workload settings",
		},
		workload: w,
	}
}
