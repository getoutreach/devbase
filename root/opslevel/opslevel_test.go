package opslevel_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/root/opslevel"
	opslevelGo "github.com/opslevel/opslevel-go/v2022"
)

func TestIsCompliant(t *testing.T) {
	client := opslevel.Client{
		LifecycleToLevelMap: opslevel.DefaultLifecycleToLevelMap,
	}
	testCases := []struct {
		name      string
		service   opslevelGo.Service
		sm        *opslevelGo.ServiceMaturity
		expected  bool
		expectErr bool
	}{
		{
			name: "level matches expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.PublicLifecycle,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: opslevel.SilverLevel,
					},
				},
			},
			expected:  true,
			expectErr: false,
		},
		{
			name: "level below expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.PublicLifecycle,
				},
				Tags: opslevelGo.TagConnection{
					Nodes: []opslevelGo.Tag{
						{
							Key:   "simple",
							Value: "true",
						},
					},
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: opslevel.BronzeLevel,
					},
				},
			},
			expected:  false,
			expectErr: false,
		},
		{
			name: "gating disabled",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.PublicLifecycle,
				},
				Tags: opslevelGo.TagConnection{
					Nodes: []opslevelGo.Tag{
						{
							Key:   "gating_disabled",
							Value: "true",
						},
					},
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: opslevel.BronzeLevel,
					},
				},
			},
			expected:  true,
			expectErr: false,
		},
		{
			name: "level above expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.PublicLifecycle,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: opslevel.GoldLevel,
					},
				},
			},
			expected:  true,
			expectErr: false,
		},
		{
			name: "lifecycle outside supported range",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 10,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: opslevel.SilverLevel,
					},
				},
			},
			expected:  false,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.IsCompliant(&tc.service, tc.sm)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %t, got: %t", tc.expected, result)
			}
		})
	}
}

func TestGetExpectedLevel(t *testing.T) {
	client := opslevel.Client{
		LifecycleToLevelMap: opslevel.DefaultLifecycleToLevelMap,
	}
	levels := []opslevelGo.Level{
		{
			Index: opslevel.BeginnerLevel,
			Name:  "Beginner",
		},
		{
			Index: opslevel.SilverLevel,
			Name:  "Silver",
		},
		{
			Index: opslevel.BronzeLevel,
			Name:  "Bronze",
		},
	}
	testCases := []struct {
		name      string
		service   opslevelGo.Service
		expected  string
		expectErr bool
	}{
		{
			name: "level matching index",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.DevelopmentLifecycle,
				},
			},
			expected:  "Bronze",
			expectErr: false,
		},
		{
			name: "level not matching index",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.PrivateBetaLifecycle,
				},
			},
			expected:  "Silver",
			expectErr: false,
		},
		{
			name: "unsupported lifecycle",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 10,
				},
			},
			expected:  "",
			expectErr: true,
		},
		{
			name: "gets last level with indexes starting at 1",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: opslevel.EndOfLifeLifecycle,
				},
			},
			expected:  "Beginner",
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.GetExpectedLevel(&tc.service, levels)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}
