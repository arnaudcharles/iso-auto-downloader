import type {ReactElement} from 'react'
import {
    siAlpinelinux,
    siDebian,
    siFedora,
    siKalilinux,
    siLinuxmint,
    siManjaro,
    siOpensuse,
    siParrotsecurity,
    siProxmox,
    siRockylinux,
    siTails,
    siTruenas,
    siUbuntu,
} from 'simple-icons'

// Providers with a real brand mark (via the CC0-licensed simple-icons set).
// Everything else gets a hand-drawn glyph (Windows, MemTest86+, Rescuezilla,
// XCP-ng, Clonezilla, MediCat) or a generic fallback below — none of those
// have a widely-recognized brand mark available in simple-icons (Windows is
// deliberately excluded there over trademark enforcement; the others just
// aren't in the set).
const BRAND_ICONS: Record<string, { path: string; hex: string }> = {
    alpinelinux: siAlpinelinux,
    debian: siDebian,
    fedora: siFedora,
    kalilinux: siKalilinux,
    linuxmint: siLinuxmint,
    manjaro: siManjaro,
    opensuse: siOpensuse,
    parrotos: siParrotsecurity,
    proxmox: siProxmox,
    rockylinux: siRockylinux,
    tails: siTails,
    truenas: siTruenas,
    ubuntu: siUbuntu,
}

const WINDOWS_PROVIDER_IDS = new Set([
    'windows11',
    'windows-server-2016',
    'windows-server-2019',
    'windows-server-2022',
    'windows-server-2025',
])

function WindowsGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="currentColor">
            <path d="M3 3.5 11 2.4V11.5H3zM12 2.3 21 1V11.5H12zM3 12.5H11V21.6L3 20.5zM12 12.5H21V23L12 21.7z"/>
        </svg>
    )
}

// RamGlyph: a RAM module — MemTest86+ tests RAM, so a memory stick reads
// more clearly than a generic disc.
function RamGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="none"
             stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="4" width="18" height="12" rx="1.5"/>
            <path d="M6 16v3M9 16v3M12 16v3M15 16v3M18 16v3"/>
            <path d="M7 8h3M7 11h3M14 8h3M14 11h3"/>
        </svg>
    )
}

// RescueCrossGlyph: a plain rescue/emergency cross — Rescuezilla is a
// disk-rescue tool. A generic cross pictogram, not any organization's
// protected emblem.
function RescueCrossGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="currentColor">
            <path d="M9 3h6v6h6v6h-6v6H9v-6H3V9h6z"/>
        </svg>
    )
}

// RocketGlyph: XCP-ng's own brand mark is a rocket ship.
function RocketGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="currentColor">
            <path d="M12 1.5c2.8 2.3 4.5 6 4.5 9.8 0 2.1-.5 3.9-1.4 5.4l.9 3.8-3-2.1-1 1.6-1-1.6-3 2.1.9-3.8C8 15.2 7.5 13.4 7.5 11.3c0-3.8 1.7-7.5 4.5-9.8z"/>
            <circle cx="12" cy="10.5" r="1.6" fill="white" fillOpacity="0.7"/>
        </svg>
    )
}

// CloneGlyph: Clonezilla has no brand mark anywhere (simple-icons or
// otherwise) — a copy/duplicate pictogram fits what the tool actually does.
function CloneGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="none"
             stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="7" width="12" height="14" rx="1.5"/>
            <path d="M8 7V5a1.5 1.5 0 0 1 1.5-1.5H19A1.5 1.5 0 0 1 20.5 5v12A1.5 1.5 0 0 1 19 18.5h-2"/>
        </svg>
    )
}

// CatGlyph: MediCat's own logo is a cat face — this is its own hand-drawn
// pictogram, not any organization's protected emblem.
function CatGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="none"
             stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M5 4.5 8.5 10h7L19 4.5v6a6.5 6.5 0 0 1-6.5 6.5h-1A6.5 6.5 0 0 1 5 10.5z"/>
            <circle cx="10" cy="11.3" r="0.9" fill="currentColor" stroke="none"/>
            <circle cx="14" cy="11.3" r="0.9" fill="currentColor" stroke="none"/>
            <path d="M11.3 13.2h1.4l-.7.8z" fill="currentColor" stroke="none"/>
            <path d="M4 11h2M4 13.3h2.3M18 11h2M17.7 13.3H20"/>
        </svg>
    )
}

function GenericIsoGlyph({className}: { className?: string }) {
    return (
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className} fill="none"
             stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="9"/>
            <circle cx="12" cy="12" r="2.5"/>
        </svg>
    )
}

// CUSTOM_ICONS covers providers with no simple-icons brand mark but a
// clearer hand-drawn alternative than the generic fallback.
const CUSTOM_ICONS: Record<string, {Glyph: (p: { className?: string }) => ReactElement, colorClassName?: string}> = {
    memtest86plus: {Glyph: RamGlyph},
    rescuezilla: {Glyph: RescueCrossGlyph, colorClassName: 'text-red-600'},
    xcpng: {Glyph: RocketGlyph},
    clonezilla: {Glyph: CloneGlyph},
    medicat: {Glyph: CatGlyph},
}

// ProviderIcon renders a small brand mark for known providers, a dedicated
// custom glyph for a handful of others (see CUSTOM_ICONS/WindowsGlyph
// above), or a generic disc glyph as a last resort so every row still gets
// an icon.
export function ProviderIcon({providerId, className}: { providerId: string; className?: string }) {
    const brand = BRAND_ICONS[providerId]
    if (brand) {
        return (
            <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" className={className}>
                <path d={brand.path} fill={`#${brand.hex}`}/>
            </svg>
        )
    }
    if (WINDOWS_PROVIDER_IDS.has(providerId)) {
        return <WindowsGlyph className={`${className ?? ''} text-orange-500`}/>
    }
    const custom = CUSTOM_ICONS[providerId]
    if (custom) {
        return <custom.Glyph className={`${className ?? ''} ${custom.colorClassName ?? 'text-orange-500'}`}/>
    }
    return <GenericIsoGlyph className={`${className ?? ''} text-orange-500`}/>
}
