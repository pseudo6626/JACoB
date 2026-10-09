//go:build windows && !nocapture

package platform

import (
	"syscall"
	"unsafe"

	"github.com/TKMAX777/winapi/dx11"
	"github.com/go-ole/go-ole"
)

const (
	wgcD3D11UsageStaging  uint32 = 3
	wgcD3D11CPUAccessRead uint32 = 0x20000
	wgcD3D11MapRead       uint32 = 1
)

type wgcDXGISampleDesc struct {
	Count   uint32
	Quality uint32
}

type wgcTexture2DDesc struct {
	Width          uint32
	Height         uint32
	MipLevels      uint32
	ArraySize      uint32
	Format         uint32
	SampleDesc     wgcDXGISampleDesc
	Usage          uint32
	BindFlags      uint32
	CPUAccessFlags uint32
	MiscFlags      uint32
}

type wgcMappedSubresource struct {
	PData      unsafe.Pointer
	RowPitch   uint32
	DepthPitch uint32
}

var (
	wgcID3D11Texture2DGUID     = ole.NewGUID("{6F15AAF2-D208-4E89-9AB4-489535D34F9C}")
	wgcDXGIInterfaceAccessGUID = ole.NewGUID("{A9B3D012-3DF2-4EE3-B8D1-8695F457D3C1}")
)

type wgcID3D11Texture2D struct {
	ole.IUnknown
}

type wgcID3D11Texture2DVtbl struct {
	ole.IUnknownVtbl
	GetDevice               uintptr
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetType                 uintptr
	SetEvictionPriority     uintptr
	GetEvictionPriority     uintptr
	GetDesc                 uintptr
}

func (v *wgcID3D11Texture2D) VTable() *wgcID3D11Texture2DVtbl {
	return (*wgcID3D11Texture2DVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *wgcID3D11Texture2D) Desc() wgcTexture2DDesc {
	var desc wgcTexture2DDesc
	syscall.SyscallN(
		v.VTable().GetDesc,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(&desc)),
	)
	return desc
}

type wgcDXGIInterfaceAccess struct {
	ole.IUnknown
}

type wgcDXGIInterfaceAccessVtbl struct {
	ole.IUnknownVtbl
	GetInterface uintptr
}

func (v *wgcDXGIInterfaceAccess) VTable() *wgcDXGIInterfaceAccessVtbl {
	return (*wgcDXGIInterfaceAccessVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *wgcDXGIInterfaceAccess) GetInterface(riid *ole.GUID, out unsafe.Pointer) error {
	r1, _, _ := syscall.SyscallN(
		v.VTable().GetInterface,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(riid)),
		uintptr(out),
	)
	if r1 != 0 {
		return ole.NewError(r1)
	}
	return nil
}

func wgcCreateTexture2D(device *dx11.ID3D11Device, desc *wgcTexture2DDesc) (*wgcID3D11Texture2D, error) {
	var tex *wgcID3D11Texture2D
	r1, _, _ := syscall.SyscallN(
		device.VTable().CreateTexture2D,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(desc)),
		0,
		uintptr(unsafe.Pointer(&tex)),
	)
	if r1 != 0 {
		return nil, ole.NewError(r1)
	}
	return tex, nil
}

type wgcD3D11Box struct {
	Left   uint32
	Top    uint32
	Front  uint32
	Right  uint32
	Bottom uint32
	Back   uint32
}

func wgcCopySubresourceRegion(
	ctx *dx11.ID3D11DeviceContext,
	dst *wgcID3D11Texture2D,
	dstSubresource, dstX, dstY, dstZ uint32,
	src *wgcID3D11Texture2D,
	srcSubresource uint32,
	srcBox *wgcD3D11Box,
) {
	syscall.SyscallN(
		ctx.VTable().CopySubresourceRegion,
		uintptr(unsafe.Pointer(ctx)),
		uintptr(unsafe.Pointer(dst)),
		uintptr(dstSubresource),
		uintptr(dstX),
		uintptr(dstY),
		uintptr(dstZ),
		uintptr(unsafe.Pointer(src)),
		uintptr(srcSubresource),
		uintptr(unsafe.Pointer(srcBox)),
	)
}

func wgcMap(ctx *dx11.ID3D11DeviceContext, resource *wgcID3D11Texture2D) (wgcMappedSubresource, error) {
	var mapped wgcMappedSubresource
	r1, _, _ := syscall.SyscallN(
		ctx.VTable().Map,
		uintptr(unsafe.Pointer(ctx)),
		uintptr(unsafe.Pointer(resource)),
		0,
		uintptr(wgcD3D11MapRead),
		0,
		uintptr(unsafe.Pointer(&mapped)),
	)
	if r1 != 0 {
		return wgcMappedSubresource{}, ole.NewError(r1)
	}
	return mapped, nil
}

func wgcUnmap(ctx *dx11.ID3D11DeviceContext, resource *wgcID3D11Texture2D) {
	syscall.SyscallN(
		ctx.VTable().Unmap,
		uintptr(unsafe.Pointer(ctx)),
		uintptr(unsafe.Pointer(resource)),
		0,
	)
}
