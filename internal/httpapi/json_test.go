package httpapi

import "testing"

func TestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "valid JSON",
			body: `{
				"user_id": "user-1",
				"driver_id": "driver-1",
				"start_point": {
					"latitude": 55.75,
					"longitude": 37.62
				},
				"end_point": {
					"latitude": 55.76,
					"longitude": 37.63
				},
				"price": 100
			}`,
			wantErr: false,
		},
		{
			name: "unknown USER_ID",
			body: `{
				"user_id": "user-1",
				"USER_ID": "user-2",
				"driver_id": "driver-1",
				"start_point": {
					"latitude": 55.75,
					"longitude": 37.62
				},
				"end_point": {
					"latitude": 55.76,
					"longitude": 37.63
				},
				"price": 100
			}`,
			wantErr: true,
		},
		{
			name: "unknown Latitude",
			body: `{
				"user_id": "user-1",
				"driver_id": "driver-1",
				"start_point": {
					"latitude": 55.75,
					"Latitude": 10,
					"longitude": 37.62
				},
				"end_point": {
					"latitude": 55.76,
					"longitude": 37.63
				},
				"price": 100
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequiredFields([]byte(tt.body))

			if (err != nil) != tt.wantErr {
				t.Errorf(
					"validateRequiredFields() error = %v, wantErr %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
