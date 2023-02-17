package s3_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhatinsights/export-service-go/s3"
)

var _ = Describe("Build files included in zip", func() {
	It("should build the meta.json file provided ExportMeta struct", func() {
		// Make the ExportMeta struct
		meta := s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters: map[string]string{
						"filter_key": "filter_value",
					},
				},
			},
			HelpString: "Help me!",
		}

		metaDump, err := s3.BuildMeta(&meta)

		Expect(err).To(BeNil())
		Expect(metaDump).To(Equal([]byte(`{"exported_by":"user","export_date":"date","export_org_id":"org_id","file_meta":[{"filename":"filename","application":"application","resource":"resource","filters":{"filter_key":"filter_value"}}],"help_string":"Help me!"}`)))

		// Error should never occur, not even for nil case
		_, err = s3.BuildMeta(nil)
		Expect(err).To(BeNil())
	})

	DescribeTable("Test the BuildReadme function", func(meta s3.ExportMeta, expected string) {
		readme, err := s3.BuildReadme(&meta)
		Expect(err).To(BeNil())

		Expect(readme).To(Equal(expected))
	},
		Entry("an empty ExportMeta struct", s3.ExportMeta{},
			`# Export Manifest

## Exported Information
- **Exported by**: 
- **Org ID**: 
- **Export Date**: 

## Data Details
This archive contains the following data:

No data was found.

## Help and Support
This service is owned by the ConsoldeDot Pipeline team. If you have any questions, or need support with this service, please contact the team on slack @crc-pipeline-team.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("No sources", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			HelpString:  "Help me!",
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

No data was found.

## Help and Support
This service is owned by the ConsoldeDot Pipeline team. If you have any questions, or need support with this service, please contact the team on slack @crc-pipeline-team.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("One source with filters", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters:     map[string]string{},
				},
			},
			HelpString: "Help me!",
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

### filename
- **Application**: application
- **Resource**: resource
- **Filters**: None

## Help and Support
This service is owned by the ConsoldeDot Pipeline team. If you have any questions, or need support with this service, please contact the team on slack @crc-pipeline-team.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("Multiple sources", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters: map[string]string{
						"filter_key": "filter_value",
					},
				},
				{
					Filename:    "filename2",
					Application: "application2",
					Resource:    "resource2",
					Filters: map[string]string{
						"filter_key":  "filter_value",
						"filter_key1": "filter_value1",
					},
				},
			},
			HelpString: "Help me!",
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

### filename
- **Application**: application
- **Resource**: resource
- **Filters**: 
  - filter_key: filter_value

### filename2
- **Application**: application2
- **Resource**: resource2
- **Filters**: 
  - filter_key: filter_value
  - filter_key1: filter_value1

## Help and Support
This service is owned by the ConsoldeDot Pipeline team. If you have any questions, or need support with this service, please contact the team on slack @crc-pipeline-team.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
	)
})
