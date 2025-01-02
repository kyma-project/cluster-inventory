package restore

type StatusType string

const (
	StatusSuccess        StatusType = "Success"
	StatusError          StatusType = "Error"
	StatusRestoreSkipped            = "Skipped"
	StatusUpdateDetected            = "UpdateDetected"
)

type RuntimeResult struct {
	RuntimeID    string     `json:"runtimeId"`
	ShootName    string     `json:"shootName"`
	Status       StatusType `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

type Results struct {
	Results         []RuntimeResult
	Succeeded       int
	Failed          int
	Skipped         int
	UpdateDetected  int
	OutputDirectory string
}

func NewRestoreResults(outputDirectory string) Results {
	return Results{
		Results:         make([]RuntimeResult, 0),
		OutputDirectory: outputDirectory,
	}
}

func (br *Results) ErrorOccurred(runtimeID, shootName string, errorMsg string) {
	result := RuntimeResult{
		RuntimeID:    runtimeID,
		ShootName:    shootName,
		Status:       StatusError,
		ErrorMessage: errorMsg,
	}

	br.Failed++
	br.Results = append(br.Results, result)
}

func (br *Results) OperationSucceeded(runtimeID string, shootName string) {
	result := RuntimeResult{
		RuntimeID: runtimeID,
		ShootName: shootName,
		Status:    StatusSuccess,
	}

	br.Succeeded++
	br.Results = append(br.Results, result)
}

func (br *Results) OperationSkipped(runtimeID string, shootName string) {
	result := RuntimeResult{
		RuntimeID: runtimeID,
		ShootName: shootName,
		Status:    StatusRestoreSkipped,
	}

	br.Skipped++
	br.Results = append(br.Results, result)
}

func (br *Results) AutomaticRestoreImpossible(runtimeID string, shootName string) {
	result := RuntimeResult{
		RuntimeID: runtimeID,
		ShootName: shootName,
		Status:    StatusUpdateDetected,
	}

	br.UpdateDetected++
	br.Results = append(br.Results, result)
}
