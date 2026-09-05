package pathpolicy

import "testing"

func TestExternalPathOnDifferentVolume(t *testing.T) {
	if !isExternalPath(`C:\workspace`, `D:\external\file.txt`) {
		t.Fatal("different-volume destination must require external approval")
	}
	if isExternalPath(`C:\workspace`, `C:\workspace\file.txt`) {
		t.Fatal("same-workspace destination must remain internal")
	}
}
