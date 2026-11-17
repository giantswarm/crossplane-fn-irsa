package main

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	xr "github.com/crossplane/function-sdk-go/resource/composite"

	"github.com/giantswarm/xfnlib/pkg/composite"
)

func TestKeepKeysFile(t *testing.T) {
	const patchTo = "status.s3Keys"

	cases := map[string]struct {
		observed map[string]any
		want     string
	}{
		"observed keys are kept": {
			observed: map[string]any{"s3Keys": "a2V5cw=="},
			want:     "a2V5cw==",
		},
		"no observed keys": {
			observed: nil,
			want:     "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			oxr := &resource.Composite{Resource: xr.New()}
			if tc.observed != nil {
				oxr.Resource.Object["status"] = tc.observed
			}
			dxr := xr.New()
			dxr.SetAPIVersion("example.com/v1")
			dxr.SetKind("XIRSA")
			composed := &composite.Composition{DesiredComposite: &resource.Composite{Resource: dxr}}

			f := &Function{log: logging.NewNopLogger()}
			if err := f.keepKeysFile(oxr, patchTo, composed); err != nil {
				t.Fatalf("keepKeysFile: %v", err)
			}

			got, _ := f.getStringFromPaved(composed.DesiredComposite.Resource, patchTo)
			if got != tc.want {
				t.Errorf("desired %s = %q, want %q", patchTo, got, tc.want)
			}
		})
	}
}
