package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func attachTestFiveMNativeBundleLoader(tb testing.TB, s *Server) {
	tb.Helper()
	if s == nil {
		tb.Fatal("server is nil")
	}
	s.fiveMNativeBundleLoader = newTestFiveMNativeBundleLoader(tb)
}

func newTestFiveMNativeBundleLoader(tb testing.TB) func(name string) ([]byte, error) {
	tb.Helper()

	bundles := map[string]string{
		"natives_universal.lua": `---@meta

---**PLAYER client**  
---[Native Documentation](https://docs.fivem.net/natives/?_0xD80958FC74E988A6)  
---Returns the entity handle for the local player ped.
---@return integer
function PlayerPedId() end

---**VEHICLE client**  
---[Native Documentation](https://docs.fivem.net/natives/?_0xA2D4EAB7A8B5E5A0)  
---Returns the maximum number of passengers supported by the vehicle model.
---@param vehicle integer
---@return integer
function GetVehicleMaxNumberOfPassengers(vehicle) end

---**CFX shared**  
---[Native Documentation](https://docs.fivem.net/natives/?_0x4D52FE5B)  
---Returns the currently invoking resource name when available.
---@return string
function GetInvokingResource() end
`,
		"natives_0193d0af.lua": `---@meta

---**VEHICLE client**  
---[Native Documentation](https://docs.fivem.net/natives/?_0xA2D4EAB7A8B5E5A0)  
---Returns the maximum number of passengers supported by the vehicle model.
---@param vehicle integer
---@return integer
function GetVehicleMaxNumberOfPassengers(vehicle) end
`,
		"natives_21e43a33.lua": `---@meta

---**VEHICLE client**  
---[Native Documentation](https://docs.fivem.net/natives/?_0xB215AAC32D25D019)  
---Returns the display name for a vehicle model.
---@param model integer
---@return string
function GetDisplayNameFromVehicleModel(model) end
`,
		"natives_server.lua": `---@meta

---**CFX server**  
---[Native Documentation](https://docs.fivem.net/natives/?_0x4D52FE5B)  
---Returns the currently invoking resource name when available.
---@return string
function GetInvokingResource() end
`,
	}

	return func(name string) ([]byte, error) {
		content, ok := bundles[name]
		if !ok {
			return nil, fmt.Errorf("missing test FiveM native bundle %s", name)
		}
		return []byte(content), nil
	}
}

func materializeTestFiveMNativeLibrary(tb testing.TB, s *Server) string {
	tb.Helper()
	if s == nil {
		tb.Fatal("server is nil")
	}
	if s.fiveMNativeBundleLoader == nil {
		tb.Fatal("FiveM native bundle loader is nil")
	}

	dir := tb.TempDir()
	for name := range fiveMNativeBundleNames {
		content, err := s.fiveMNativeBundleLoader(name)
		if err != nil {
			tb.Fatalf("load test FiveM native bundle %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			tb.Fatalf("write test FiveM native bundle %s: %v", name, err)
		}
	}

	return dir
}
