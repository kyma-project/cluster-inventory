package restore

import (
	authenticationv1alpha1 "github.com/gardener/oidc-webhook-authenticator/apis/authentication/v1alpha1"
	v12 "k8s.io/api/rbac/v1"
)

type StatusType string

const (
	StatusSuccess        StatusType = "Success"
	StatusError          StatusType = "Error"
	StatusUpdateDetected            = "UpdateDetected"
)

type RuntimeResult struct {
	RuntimeID     string     `json:"runtimeId"`
	ShootName     string     `json:"shootName"`
	Status        StatusType `json:"status"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	RestoredCRBs  []string   `json:"restoredCRBs,omitempty"`
	RestoredOIDCs []string   `json:"restoredOIDCs,omitempty"`
}

type Results struct {
	Results         []RuntimeResult
	Succeeded       int
	Failed          int
	UpdateDetected  int
	OutputDirectory string
}

func NewRestoreResults(outputDirectory string) Results {
	return Results{
		Results:         make([]RuntimeResult, 0),
		OutputDirectory: outputDirectory,
	}
}

func (rr *Results) ErrorOccurred(runtimeID, shootName string, errorMsg string) {
	result := RuntimeResult{
		RuntimeID:    runtimeID,
		ShootName:    shootName,
		Status:       StatusError,
		ErrorMessage: errorMsg,
	}

	rr.Failed++
	rr.Results = append(rr.Results, result)
}

func (rr *Results) OperationSucceeded(runtimeID string, shootName string, appliedCRBs []v12.ClusterRoleBinding, appliedOIDCs []authenticationv1alpha1.OpenIDConnect) {

	var appliedCRBsString []string
	for _, crb := range appliedCRBs {
		appliedCRBsString = append(appliedCRBsString, crb.Name)
	}

	var appliedOIDCsString []string
	for _, oidc := range appliedOIDCs {
		appliedOIDCsString = append(appliedOIDCsString, oidc.Name)
	}

	result := RuntimeResult{
		RuntimeID:     runtimeID,
		ShootName:     shootName,
		Status:        StatusSuccess,
		RestoredCRBs:  appliedCRBsString,
		RestoredOIDCs: appliedOIDCsString,
	}

	rr.Succeeded++
	rr.Results = append(rr.Results, result)
}

func (rr *Results) AutomaticRestoreImpossible(runtimeID string, shootName string) {
	result := RuntimeResult{
		RuntimeID: runtimeID,
		ShootName: shootName,
		Status:    StatusUpdateDetected,
	}

	rr.UpdateDetected++
	rr.Results = append(rr.Results, result)
}
