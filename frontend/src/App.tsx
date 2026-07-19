import {useEffect, useState} from 'react'
import {
    CheckAll,
    CheckOne,
    ChooseDownloadDir,
    DownloadAll,
    DownloadDir,
    DownloadOne,
    ListProviders,
    ListProviderVariants,
    RescanFound,
    SetAllVariantsEnabled,
    SetTheme,
    SetVariantEnabled,
    Theme as GetTheme,
    ValidateAll,
    ValidateOne,
} from '../wailsjs/go/main/App'
import {EventsOn} from '../wailsjs/runtime/runtime'
import {main} from '../wailsjs/go/models'
import {ProviderIcon} from './icons'
import logo from './assets/logo.png'

type VariantStatus = main.VariantStatus
type ProviderInfo = main.ProviderInfo
type VariantSetting = main.VariantSetting

const STATUS_LABEL: Record<string, string> = {
    'unknown': 'Not checked',
    'checking': 'Checking…',
    'up-to-date': 'Up to date',
    'outdated': 'Outdated',
    'not-found': 'Not found',
    'downloading': 'Downloading…',
    'validating': 'Validating…',
    'not-tested': 'Not Tested',
    'error': 'Error',
}

const STATUS_CLASSES: Record<string, string> = {
    'unknown': 'bg-slate-200 text-slate-700 dark:bg-neutral-700 dark:text-neutral-200',
    'checking': 'bg-sky-100 text-sky-700 dark:bg-sky-900 dark:text-sky-200 animate-pulse',
    'up-to-date': 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900 dark:text-emerald-200',
    'outdated': 'bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-200',
    'not-found': 'bg-slate-200 text-slate-700 dark:bg-neutral-700 dark:text-neutral-200',
    'downloading': 'bg-sky-100 text-sky-700 dark:bg-sky-900 dark:text-sky-200',
    'validating': 'bg-violet-100 text-violet-700 dark:bg-violet-900 dark:text-violet-200 animate-pulse',
    'not-tested': 'bg-slate-100 text-slate-400 dark:bg-neutral-800 dark:text-neutral-500',
    'error': 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200',
}

function SunIcon() {
    return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
             strokeLinecap="round" strokeLinejoin="round" className="h-5 w-5">
            <circle cx="12" cy="12" r="4"/>
            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>
        </svg>
    )
}

function MoonIcon() {
    return (
        <svg viewBox="0 0 24 24" fill="currentColor" className="h-5 w-5">
            <path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/>
        </svg>
    )
}

function StatusBadge({status}: { status: string }) {
    const cls = STATUS_CLASSES[status] ?? STATUS_CLASSES['unknown']
    const label = STATUS_LABEL[status] ?? status
    return (
        <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`}>
            {label}
        </span>
    )
}

function VariantRow({provider, variant, onChanged}: {
    provider: ProviderInfo
    variant: VariantStatus
    onChanged: () => void
}) {
    const busy = variant.status === 'checking' || variant.status === 'downloading' || variant.status === 'validating'
    const canDownload = variant.status === 'outdated' || variant.status === 'not-found'
    const canValidate = variant.foundVersion !== ''

    return (
        <div className="flex items-center justify-between gap-4 py-3">
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                    <span className="font-medium text-slate-900 dark:text-neutral-100">{variant.label}</span>
                    <StatusBadge status={variant.status}/>
                </div>
                <div className="mt-1 text-sm text-slate-500 dark:text-neutral-400">
                    {variant.foundVersion ? `Found: ${variant.foundVersion}` : 'Not found'}
                    {variant.latestVersion ? ` · Latest: ${variant.latestVersion}` : ''}
                </div>
                {variant.status === 'downloading' && (
                    <div className="mt-2 h-1.5 w-full max-w-xs overflow-hidden rounded-full bg-slate-200 dark:bg-neutral-700">
                        <div
                            className="h-full rounded-full bg-sky-500 transition-all"
                            style={{width: `${Math.max(4, Math.round(variant.progress * 100))}%`}}
                        />
                    </div>
                )}
                {variant.status === 'error' && variant.error && (
                    <div className="mt-1 text-sm text-red-600 dark:text-red-400">{variant.error}</div>
                )}
                {variant.status === 'not-tested' && variant.error && (
                    <div className="mt-1 text-sm text-slate-400 dark:text-neutral-500">{variant.error}</div>
                )}
            </div>

            <div className="flex shrink-0 gap-2">
                <button
                    disabled={busy}
                    onClick={() => {
                        CheckOne(provider.id, variant.variantIndex).then(onChanged)
                    }}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                >
                    Check
                </button>
                <button
                    disabled={busy || !canValidate}
                    title={canValidate ? 'Re-verify the downloaded file against its published checksum' : 'Nothing downloaded yet to validate'}
                    onClick={() => {
                        ValidateOne(provider.id, variant.variantIndex).then(onChanged)
                    }}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                >
                    Validation
                </button>
                <button
                    disabled={busy || !canDownload}
                    onClick={() => {
                        DownloadOne(provider.id, variant.variantIndex).then(onChanged)
                    }}
                    className="rounded-md bg-orange-400 px-3 py-1.5 text-sm font-medium text-white hover:bg-orange-500 disabled:opacity-50 dark:bg-orange-400 dark:hover:bg-orange-500"
                >
                    {variant.status === 'outdated' ? 'New version available' : 'Download'}
                </button>
            </div>
        </div>
    )
}

function SettingsPanel({downloadDir, onDownloadDirChanged, onProvidersChanged}: {
    downloadDir: string
    onDownloadDirChanged: (dir: string) => void
    onProvidersChanged: () => void
}) {
    const [variants, setVariants] = useState<VariantSetting[]>([])
    const [error, setError] = useState('')

    const refreshSettings = () => {
        ListProviderVariants().then(setVariants)
    }

    useEffect(refreshSettings, [])

    const byProvider = new Map<string, VariantSetting[]>()
    for (const v of variants) {
        const list = byProvider.get(v.providerId) ?? []
        list.push(v)
        byProvider.set(v.providerId, list)
    }
    const allEnabled = variants.length > 0 && variants.every(v => v.enabled)

    return (
        <div className="rounded-lg border border-slate-200 px-4 py-4 dark:border-neutral-800">
            <div className="mb-4 flex items-center justify-between gap-4">
                <div className="min-w-0">
                    <div className="text-sm font-semibold text-slate-900 dark:text-neutral-100">Destination</div>
                    <div className="truncate text-sm text-slate-500 dark:text-neutral-400">{downloadDir || '…'}</div>
                </div>
                <div className="flex shrink-0 gap-2">
                    <button
                        onClick={() => {
                            setError('')
                            RescanFound().then(onProvidersChanged).catch(err => setError(String(err)))
                        }}
                        title="Re-scan the destination folder for ISOs already there"
                        className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                    >
                        Rescan
                    </button>
                    <button
                        onClick={() => {
                            setError('')
                            ChooseDownloadDir().then(onDownloadDirChanged).catch(err => setError(String(err)))
                        }}
                        className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                    >
                        Choose…
                    </button>
                </div>
            </div>
            {error && <div className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</div>}

            <div className="flex items-center justify-between gap-4">
                <div className="text-sm font-semibold text-slate-900 dark:text-neutral-100">Available ISOs</div>
                <button
                    onClick={() => {
                        setError('')
                        SetAllVariantsEnabled(!allEnabled).then(() => {
                            refreshSettings()
                            onProvidersChanged()
                        }).catch(err => setError(String(err)))
                    }}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                >
                    {allEnabled ? 'Uncheck all' : 'Check all'}
                </button>
            </div>
            <p className="mt-1 text-sm text-slate-500 dark:text-neutral-400">
                Everything is checked by default. Uncheck any ISO you're not interested in — it
                disappears from the main screen.
            </p>
            <div className="mt-2 divide-y divide-slate-200 dark:divide-neutral-800">
                {[...byProvider.entries()].map(([providerId, rows]) => (
                    <div key={providerId} className="py-2">
                        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-orange-600 dark:text-orange-400">
                            <ProviderIcon providerId={providerId} className="h-3.5 w-3.5 shrink-0"/>
                            {rows[0].providerName}
                            <span className="font-normal normal-case text-slate-400 dark:text-neutral-500">
                                · {rows[0].category}
                            </span>
                        </div>
                        {rows.map(v => (
                            <label
                                key={v.variantIndex}
                                className="flex cursor-pointer items-center justify-between gap-4 py-1 pl-5"
                            >
                                <span className="text-sm text-slate-700 dark:text-neutral-300">{v.label}</span>
                                <input
                                    type="checkbox"
                                    checked={v.enabled}
                                    onChange={e => {
                                        setError('')
                                        SetVariantEnabled(v.providerId, v.variantIndex, e.target.checked).then(() => {
                                            refreshSettings()
                                            onProvidersChanged()
                                        }).catch(err => setError(String(err)))
                                    }}
                                    className="h-4 w-4 rounded border-slate-300 accent-orange-500 dark:border-neutral-600"
                                />
                            </label>
                        ))}
                    </div>
                ))}
            </div>
        </div>
    )
}

function App() {
    const [providers, setProviders] = useState<ProviderInfo[]>([])
    const [downloadDir, setDownloadDir] = useState('')
    const [checkingAll, setCheckingAll] = useState(false)
    const [downloadingAll, setDownloadingAll] = useState(false)
    const [validatingAll, setValidatingAll] = useState(false)
    const [settingsOpen, setSettingsOpen] = useState(false)
    const [dark, setDark] = useState(false)

    const refresh = () => {
        ListProviders().then(setProviders)
    }

    useEffect(() => {
        refresh()
        DownloadDir().then(setDownloadDir)

        GetTheme().then(theme => {
            const isDark = theme === 'dark' ||
                (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
            setDark(isDark)
            document.documentElement.classList.toggle('dark', isDark)
        })

        const unsubscribe = EventsOn('provider:status', () => {
            refresh()
        })
        return unsubscribe
    }, [])

    const toggleTheme = () => {
        const next = !dark
        setDark(next)
        document.documentElement.classList.toggle('dark', next)
        SetTheme(next ? 'dark' : 'light')
    }

    return (
        <div className="min-h-full bg-white text-slate-900 dark:bg-neutral-900 dark:text-neutral-100">
            <header className="flex items-center justify-between border-b border-slate-200 px-6 py-4 dark:border-neutral-800">
                <div className="flex items-center gap-3">
                    <img src={logo} alt="" className="h-9 w-9 rounded-md"/>
                    <div>
                        <h1 className="text-lg font-semibold">ISO Auto Downloader</h1>
                        <p className="text-sm text-slate-500 dark:text-neutral-400">
                            Saving to {downloadDir || '…'}
                        </p>
                    </div>
                </div>
                <div className="flex gap-2">
                    <button
                        onClick={toggleTheme}
                        aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
                        className="rounded-md border border-slate-300 p-2 text-slate-700 hover:bg-slate-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                    >
                        {dark ? <SunIcon/> : <MoonIcon/>}
                    </button>
                    <button
                        onClick={() => setSettingsOpen(o => !o)}
                        aria-pressed={settingsOpen}
                        className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                    >
                        {settingsOpen ? '← Back' : 'Settings'}
                    </button>
                    {!settingsOpen && (
                        <>
                            <button
                                disabled={checkingAll}
                                onClick={() => {
                                    setCheckingAll(true)
                                    CheckAll().finally(() => setTimeout(() => setCheckingAll(false), 500))
                                }}
                                className="rounded-md border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                            >
                                {checkingAll ? 'Checking…' : 'Check all'}
                            </button>
                            <button
                                disabled={validatingAll}
                                title="Re-verify every downloaded file against its published checksum"
                                onClick={() => {
                                    setValidatingAll(true)
                                    ValidateAll().finally(() => setTimeout(() => setValidatingAll(false), 500))
                                }}
                                className="rounded-md border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-neutral-600 dark:text-neutral-200 dark:hover:bg-neutral-800"
                            >
                                {validatingAll ? 'Validating…' : 'Validate All'}
                            </button>
                            <button
                                disabled={downloadingAll}
                                onClick={() => {
                                    setDownloadingAll(true)
                                    DownloadAll().finally(() => setTimeout(() => setDownloadingAll(false), 500))
                                }}
                                className="rounded-md bg-orange-400 px-4 py-2 text-sm font-medium text-white hover:bg-orange-500 disabled:opacity-50"
                            >
                                {downloadingAll ? 'Downloading…' : 'Download All'}
                            </button>
                        </>
                    )}
                </div>
            </header>

            <main className="mx-auto max-w-3xl px-6 py-6">
                {settingsOpen ? (
                    <SettingsPanel
                        downloadDir={downloadDir}
                        onDownloadDirChanged={setDownloadDir}
                        onProvidersChanged={refresh}
                    />
                ) : (
                    <>
                        {providers.length === 0 && (
                            <div className="flex flex-col items-center justify-center py-24">
                                <img src={logo} alt="" className="h-40 w-40 opacity-20"/>
                                <p className="mt-4 text-sm text-slate-400 dark:text-neutral-600">
                                    Every ISO is unchecked — open Settings to bring some back.
                                </p>
                            </div>
                        )}
                        {providers.map(provider => (
                            <section key={provider.id} className="mb-6">
                                <h2 className="mb-1 flex items-center gap-1.5 text-sm font-semibold uppercase tracking-wide text-orange-600 dark:text-orange-400">
                                    <ProviderIcon providerId={provider.id} className="h-4 w-4 shrink-0"/>
                                    {provider.name}
                                </h2>
                                <div className="divide-y divide-slate-200 rounded-lg border border-slate-200 px-4 dark:divide-neutral-800 dark:border-neutral-800">
                                    {provider.variants.map(variant => (
                                        <VariantRow
                                            key={variant.variantIndex}
                                            provider={provider}
                                            variant={variant}
                                            onChanged={refresh}
                                        />
                                    ))}
                                </div>
                            </section>
                        ))}
                    </>
                )}
            </main>
        </div>
    )
}

export default App
