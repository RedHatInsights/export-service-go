package s3

import (
	"encoding/json"
	"fmt"

	"github.com/redhatinsights/export-service-go/models"
)

/**
 * Code for making the README.md and meta.json files included in the tar file.
 */

// struct used to fill the README.md and meta.json files in the tar
type ExportMeta struct {
	ExportBy    string           `json:"exported_by"`
	ExportDate  string           `json:"export_date"`
	ExportOrgID string           `json:"export_org_id"`
	FileMeta    []ExportFileMeta `json:"file_meta"`
	HelpString  string           `json:"help_string"`
	FailedFiles []FailedFileMeta `json:"failed_files,omitempty"`
}

// details for each file in the tar
type ExportFileMeta struct {
	Filename    string `json:"filename"`
	Application string `json:"application"`
	Resource    string `json:"resource"`
	// Filters are a key-value pair of the filters used to create the export
	Filters map[string]string `json:"filters"`
}

type FailedFileMeta struct {
	Filename    string      `json:"filename"`
	Application string      `json:"application"`
	Resource    string      `json:"resource"`
	Error       ExportError `json:"error"`
}

type ExportError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	helpString = `Contained in this archive are your requested resources. If you need help or have any questions, please contact Red Hat Support.`
)

func findFileMeta(id, basename string, sources []models.Source) (*ExportFileMeta, error) {
	for _, source := range sources {
		if source.ID.String() == id {
			filters, err := source.GetFilters()
			if err != nil {
				return nil, err
			}

			return &ExportFileMeta{
				Filename:    basename,
				Application: source.Application,
				Resource:    source.Resource,
				Filters:     filters,
			}, nil
		}
	}
	return nil, nil
}

func BuildMeta(meta *ExportMeta) ([]byte, error) {
	// make a json file from the ExportMeta struct
	metaJSON, err := json.Marshal(meta)

	return metaJSON, err
}

func BuildReadme(meta *ExportMeta) (string, error) {
	dataDetails := ""
	for _, file := range meta.FileMeta {
		filterDetails := ""
		for key, value := range file.Filters {
			filterDetails += fmt.Sprintf(
				`
  - %s: %s`,
				key, value)
		}

		if filterDetails == "" {
			filterDetails = "None"
		}
		dataDetails += fmt.Sprintf(`
### %s
- **Application**: %s
- **Resource**: %s
- **Filters**: %s
`, file.Filename, file.Application, file.Resource, filterDetails)
	}

	if dataDetails == "" {
		dataDetails = `
No data was found.
`
	}

	failedFilesDetails := ""
	for _, failedFile := range meta.FailedFiles {
		failedFilesDetails += fmt.Sprintf(`
### %s (Failed)
- **Application**: %s
- **Resource**: %s
- **Error Code**: %s
- **Error Message**: %s
`, failedFile.Filename, failedFile.Application, failedFile.Resource, failedFile.Error.Code, failedFile.Error.Message)
	}
	if failedFilesDetails == "" {
		failedFilesDetails = `
No failures reported.
`
	}

	// next, make a README.md file containing the ExportMeta data in a readable format
	readme := fmt.Sprintf(`# Export Manifest

## Exported Information
- **Exported by**: %s
- **Org ID**: %s
- **Export Date**: %s

## Data Details
This archive contains the following data:
%s

## Failed Files
%s

## Help and Support
If you have any questions, or need support with this service, please contact Red Hat Support or visit the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		meta.ExportBy,
		meta.ExportOrgID,
		meta.ExportDate,
		dataDetails,
		failedFilesDetails,
	)

	return readme, nil
}
