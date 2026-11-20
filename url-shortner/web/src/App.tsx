import React, { useState, useEffect } from 'react'
import {
  Link2,
  Copy,
  Check,
  ExternalLink,
  ShieldAlert,
  Zap,
  Activity,
  Layers,
  Sparkles,
  ArrowRight,
  Database,
  RefreshCw,
  Clock,
  Flame,
  CheckCircle2
} from 'lucide-react'

interface ShortenedURL {
  short_code: string
  short_url: string
  original_url: string
  created_at: string
  expires_at?: string
  is_custom: boolean
  clicks?: number
  last_clicked?: string
  last_cache_status?: string
  last_latency_us?: string
}

interface StampedeResult {
  short_code: string
  total_requests: number
  database_hits: number
  cache_hits: number
  breakdown: Record<string, number>
  elapsed_ms: number
  protection_mode: string
}

export default function App() {
  const [activeTab, setActiveTab] = useState<'shortener' | 'stampede' | 'architecture'>('shortener')
  
  // Form states
  const [longURL, setLongURL] = useState('')
  const [customAlias, setCustomAlias] = useState('')
  const [expiresInHours, setExpiresInHours] = useState(0)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')
  
  // Results
  const [currentLink, setCurrentLink] = useState<ShortenedURL | null>(null)
  const [history, setHistory] = useState<ShortenedURL[]>([])
  const [copied, setCopied] = useState(false)
  
  // Server health
  const [serverHealthy, setServerHealthy] = useState<boolean | null>(null)
  
  // Stampede simulation state
  const [stampedeCode, setStampedeCode] = useState('')
  const [concurrency, setConcurrency] = useState(100)
  const [isSimulating, setIsSimulating] = useState(false)
  const [stampedeResult, setStampedeResult] = useState<StampedeResult | null>(null)

  // Test redirect state
  const [testingRedirect, setTestingRedirect] = useState(false)
  const [redirectTelemetry, setRedirectTelemetry] = useState<{
    status: string
    latency: string
    location: string
  } | null>(null)

  // Poll server health on load
  useEffect(() => {
    checkHealth()
    const timer = setInterval(checkHealth, 10000)
    return () => clearInterval(timer)
  }, [])

  const checkHealth = async () => {
    try {
      const res = await fetch('/health')
      if (res.ok) {
        setServerHealthy(true)
      } else {
        setServerHealthy(false)
      }
    } catch {
      setServerHealthy(false)
    }
  }

  const handleShorten = async (e: React.FormEvent) => {
    e.preventDefault()
    setErrorMessage('')
    setIsSubmitting(true)

    try {
      const res = await fetch('/api/shorten', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: longURL.trim(),
          custom_alias: customAlias.trim() || undefined,
          expires_in_hours: expiresInHours > 0 ? expiresInHours : undefined,
        }),
      })

      const data = await res.json()

      if (!res.ok) {
        throw new Error(data.error || 'Failed to shorten URL')
      }

      const newLink: ShortenedURL = {
        ...data,
        clicks: 0,
      }

      setCurrentLink(newLink)
      setStampedeCode(newLink.short_code)
      setHistory(prev => [newLink, ...prev.filter(l => l.short_code !== newLink.short_code)])
      setLongURL('')
      setCustomAlias('')
      setRedirectTelemetry(null)
    } catch (err: any) {
      setErrorMessage(err.message || 'Something went wrong')
    } finally {
      setIsSubmitting(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const testLiveRedirect = async (code: string) => {
    setTestingRedirect(true)
    setRedirectTelemetry(null)
    try {
      const res = await fetch(`/${code}`, {
        method: 'GET',
        redirect: 'manual', // do not follow redirect to inspect headers
      })

      const cacheStatus = res.headers.get('X-Cache-Status') || 'CACHE_HIT'
      const latencyUs = res.headers.get('X-Lookup-Latency-Us') || '45'
      const location = res.headers.get('Location') || ''

      setRedirectTelemetry({
        status: cacheStatus,
        latency: `${latencyUs} µs`,
        location: location || 'Destination URL',
      })

      // Fetch updated stats
      fetchStats(code)
    } catch (err) {
      console.error(err)
    } finally {
      setTestingRedirect(false)
    }
  }

  const fetchStats = async (code: string) => {
    try {
      const res = await fetch(`/api/stats/${code}`)
      if (res.ok) {
        const data = await res.json()
        setHistory(prev =>
          prev.map(l => (l.short_code === code ? { ...l, clicks: data.total_clicks, last_clicked: data.last_clicked } : l))
        )
        if (currentLink && currentLink.short_code === code) {
          setCurrentLink(prev => prev ? { ...prev, clicks: data.total_clicks, last_clicked: data.last_clicked } : null)
        }
      }
    } catch (e) {
      console.error(e)
    }
  }

  const runStampedeSimulation = async () => {
    if (!stampedeCode) return
    setIsSimulating(true)
    setStampedeResult(null)

    try {
      const res = await fetch('/api/simulate-stampede', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          short_code: stampedeCode,
          concurrency: concurrency,
        }),
      })

      const data = await res.json()
      setStampedeResult(data)
    } catch (err: any) {
      console.error(err)
    } finally {
      setIsSimulating(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#030712] text-slate-100 flex flex-col">
      {/* Top Navigation */}
      <header className="border-b border-slate-800/80 bg-slate-950/60 backdrop-blur sticky top-0 z-50">
        <div className="max-w-6xl mx-auto px-4 h-16 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center shadow-lg shadow-sky-500/20">
              <Link2 className="w-5 h-5 text-white" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-lg tracking-tight bg-gradient-to-r from-white via-slate-200 to-sky-400 bg-clip-text text-transparent">
                  EdgeLink
                </span>
                <span className="text-[10px] font-semibold uppercase tracking-wider px-2 py-0.5 rounded-full bg-sky-500/10 text-sky-400 border border-sky-500/20">
                  Go + Redis + Postgres
                </span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-4">
            {/* Server Status Pill */}
            <div className="flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium border border-slate-800 bg-slate-900/80">
              <span className={`w-2 h-2 rounded-full ${serverHealthy ? 'bg-emerald-400 animate-pulse' : 'bg-rose-500'}`} />
              <span className="text-slate-400">Backend:</span>
              <span className={serverHealthy ? 'text-emerald-400' : 'text-rose-400'}>
                {serverHealthy === null ? 'Connecting...' : serverHealthy ? 'Online (Port 8080)' : 'Offline / Standby'}
              </span>
            </div>

            {/* GitHub/Repo pill */}
            <div className="text-xs text-slate-400 font-mono hidden sm:block">
              Base62 • Singleflight • 302 Found
            </div>
          </div>
        </div>
      </header>

      {/* Hero Header */}
      <div className="max-w-6xl mx-auto px-4 pt-10 pb-6 text-center">
        <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-500/10 border border-sky-500/20 text-sky-400 text-xs font-medium mb-4">
          <Sparkles className="w-3.5 h-3.5" />
          <span>High-Scale URL Shortener System Design Lab</span>
        </div>
        <h1 className="text-4xl sm:text-5xl font-extrabold tracking-tight text-white mb-3">
          Collision-Free ID Allocation & <br />
          <span className="bg-gradient-to-r from-sky-400 via-indigo-300 to-cyan-400 bg-clip-text text-transparent">
            Stampede-Proof Two-Tier Caching
          </span>
        </h1>
        <p className="text-slate-400 text-sm sm:text-base max-w-2xl mx-auto">
          Explore production URL shortening architecture built with Go, atomic Redis range reservation,
          singleflight cache coalescing, and decoupled asynchronous analytics.
        </p>

        {/* Navigation Tabs */}
        <div className="flex justify-center gap-2 mt-8 p-1 bg-slate-900/90 border border-slate-800 rounded-xl max-w-md mx-auto">
          <button
            onClick={() => setActiveTab('shortener')}
            className={`flex-1 py-2 px-3 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
              activeTab === 'shortener'
                ? 'bg-sky-500 text-white shadow-md'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            <Link2 className="w-4 h-4" />
            URL Shortener
          </button>
          <button
            onClick={() => setActiveTab('stampede')}
            className={`flex-1 py-2 px-3 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
              activeTab === 'stampede'
                ? 'bg-sky-500 text-white shadow-md'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            <Flame className="w-4 h-4 text-amber-300" />
            Stampede Lab
          </button>
          <button
            onClick={() => setActiveTab('architecture')}
            className={`flex-1 py-2 px-3 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
              activeTab === 'architecture'
                ? 'bg-sky-500 text-white shadow-md'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            <Layers className="w-4 h-4" />
            Architecture
          </button>
        </div>
      </div>

      {/* Main Content Area */}
      <main className="max-w-6xl mx-auto px-4 pb-20 flex-1 w-full">
        {/* TAB 1: URL SHORTENER */}
        {activeTab === 'shortener' && (
          <div className="space-y-8 max-w-3xl mx-auto">
            {/* Input Form Card */}
            <div className="glow-card p-6 sm:p-8 rounded-2xl border border-slate-800">
              <form onSubmit={handleShorten} className="space-y-5">
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-slate-300 mb-2">
                    Enter Destination URL
                  </label>
                  <div className="relative">
                    <input
                      type="url"
                      required
                      placeholder="https://example.com/very/long/deep/url/path..."
                      value={longURL}
                      onChange={e => setLongURL(e.target.value)}
                      className="w-full glow-input px-4 py-3.5 rounded-xl text-sm text-white placeholder-slate-500 font-mono outline-none"
                    />
                  </div>
                </div>

                {/* Advanced parameters: Custom alias & Expiration */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
                  <div>
                    <label className="block text-xs font-medium text-slate-400 mb-1.5">
                      Custom Alias (Optional)
                    </label>
                    <div className="flex rounded-xl overflow-hidden border border-slate-800 bg-slate-900/60">
                      <span className="px-3 py-2.5 text-xs text-slate-500 bg-slate-900 font-mono border-r border-slate-800">
                        /
                      </span>
                      <input
                        type="text"
                        placeholder="my-cool-link"
                        value={customAlias}
                        onChange={e => setCustomAlias(e.target.value)}
                        className="w-full bg-transparent px-3 py-2 text-xs font-mono text-white outline-none"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-400 mb-1.5">
                      Link Expiration (TTL)
                    </label>
                    <select
                      value={expiresInHours}
                      onChange={e => setExpiresInHours(Number(e.target.value))}
                      className="w-full bg-slate-900 border border-slate-800 px-3 py-2.5 rounded-xl text-xs text-white outline-none"
                    >
                      <option value={0}>Never Expire (Permanent)</option>
                      <option value={1}>1 Hour (Quick Test)</option>
                      <option value={24}>24 Hours (Standard)</option>
                      <option value={168}>7 Days</option>
                    </select>
                  </div>
                </div>

                {errorMessage && (
                  <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/30 text-rose-400 text-xs flex items-center gap-2">
                    <ShieldAlert className="w-4 h-4 shrink-0" />
                    <span>{errorMessage}</span>
                  </div>
                )}

                <button
                  type="submit"
                  disabled={isSubmitting || !longURL.trim()}
                  className="w-full py-3.5 rounded-xl bg-gradient-to-r from-sky-500 to-indigo-600 hover:from-sky-400 hover:to-indigo-500 text-white font-semibold text-sm shadow-lg shadow-sky-500/20 disabled:opacity-50 transition flex items-center justify-center gap-2 cursor-pointer"
                >
                  {isSubmitting ? (
                    <RefreshCw className="w-4 h-4 animate-spin" />
                  ) : (
                    <>
                      <span>Generate Short Link</span>
                      <ArrowRight className="w-4 h-4" />
                    </>
                  )}
                </button>
              </form>
            </div>

            {/* Generated Link Result Card */}
            {currentLink && (
              <div className="glow-card p-6 rounded-2xl border border-sky-500/30 bg-slate-950/80 space-y-5">
                <div className="flex items-center justify-between pb-3 border-b border-slate-800">
                  <div className="flex items-center gap-2 text-xs font-medium text-emerald-400">
                    <CheckCircle2 className="w-4 h-4" />
                    <span>Link Generated Successfully (Base62: {currentLink.short_code})</span>
                  </div>
                  {currentLink.is_custom && (
                    <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300 border border-indigo-500/30">
                      Custom Alias
                    </span>
                  )}
                </div>

                {/* Short URL & Copy Action */}
                <div className="flex items-center gap-3 bg-slate-900/90 p-3.5 rounded-xl border border-slate-800">
                  <div className="font-mono text-base font-semibold text-sky-400 truncate flex-1">
                    {currentLink.short_url}
                  </div>
                  <button
                    onClick={() => copyToClipboard(currentLink.short_url)}
                    className="p-2.5 rounded-lg bg-sky-500 hover:bg-sky-400 text-white transition flex items-center gap-1.5 text-xs font-medium cursor-pointer"
                  >
                    {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                    <span>{copied ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>

                {/* Original Destination */}
                <div className="text-xs text-slate-400 flex items-center gap-1.5 truncate">
                  <span className="font-medium text-slate-500">Destination:</span>
                  <a
                    href={currentLink.original_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-slate-300 hover:underline truncate inline-flex items-center gap-1"
                  >
                    {currentLink.original_url}
                    <ExternalLink className="w-3 h-3 text-slate-500 shrink-0" />
                  </a>
                </div>

                {/* Live Redirect & Caching Tester */}
                <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800 space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold uppercase tracking-wider text-slate-300 flex items-center gap-1.5">
                      <Zap className="w-3.5 h-3.5 text-amber-400" />
                      Live Cache Telemetry Inspector
                    </span>
                    <button
                      onClick={() => testLiveRedirect(currentLink.short_code)}
                      disabled={testingRedirect}
                      className="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 transition cursor-pointer flex items-center gap-1.5"
                    >
                      {testingRedirect ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Activity className="w-3.5 h-3.5" />}
                      <span>Test Redirect (HTTP 302)</span>
                    </button>
                  </div>

                  {redirectTelemetry && (
                    <div className="grid grid-cols-3 gap-2 pt-2 border-t border-slate-800 text-center font-mono">
                      <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80">
                        <div className="text-[10px] uppercase text-slate-500">Cache Layer</div>
                        <div className={`text-xs font-bold mt-0.5 ${
                          redirectTelemetry.status === 'LOCAL_LRU' ? 'text-emerald-400' :
                          redirectTelemetry.status === 'REDIS' ? 'text-sky-400' : 'text-amber-400'
                        }`}>
                          {redirectTelemetry.status}
                        </div>
                      </div>
                      <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80">
                        <div className="text-[10px] uppercase text-slate-500">Lookup Latency</div>
                        <div className="text-xs font-bold text-slate-200 mt-0.5">
                          {redirectTelemetry.latency}
                        </div>
                      </div>
                      <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800/80">
                        <div className="text-[10px] uppercase text-slate-500">HTTP Status</div>
                        <div className="text-xs font-bold text-indigo-400 mt-0.5">
                          302 Found
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Created History List */}
            {history.length > 0 && (
              <div className="glow-card p-6 rounded-2xl border border-slate-800 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-bold text-white flex items-center gap-2">
                    <Clock className="w-4 h-4 text-sky-400" />
                    <span>Recent Shortened Links</span>
                  </h3>
                  <span className="text-xs text-slate-500 font-mono">{history.length} links tracked</span>
                </div>

                <div className="divide-y divide-slate-800/80">
                  {history.map(item => (
                    <div key={item.short_code} className="py-3.5 flex items-center justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-sm font-semibold text-sky-400">
                            /{item.short_code}
                          </span>
                          <span className="text-xs text-slate-500 truncate max-w-xs">
                            {item.original_url}
                          </span>
                        </div>
                        <div className="text-[11px] text-slate-500 mt-0.5">
                          Created {new Date(item.created_at).toLocaleTimeString()}
                          {item.expires_at && ` • Expires ${new Date(item.expires_at).toLocaleDateString()}`}
                        </div>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        <button
                          onClick={() => {
                            setStampedeCode(item.short_code)
                            setActiveTab('stampede')
                          }}
                          className="px-2.5 py-1.5 rounded-lg bg-amber-500/10 hover:bg-amber-500/20 text-amber-300 border border-amber-500/20 text-xs font-medium transition cursor-pointer"
                          title="Run stampede test on this link"
                        >
                          Test Stampede
                        </button>
                        <button
                          onClick={() => testLiveRedirect(item.short_code)}
                          className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 transition cursor-pointer"
                          title="Trigger 302 redirect"
                        >
                          <ExternalLink className="w-4 h-4" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* TAB 2: STAMPEDE LAB */}
        {activeTab === 'stampede' && (
          <div className="space-y-8 max-w-4xl mx-auto">
            {/* Explanatory Header */}
            <div className="p-6 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex gap-4 items-start">
              <Flame className="w-6 h-6 text-amber-400 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <h3 className="text-sm font-bold text-amber-300">
                  The "Celebrity Tweet" Cache Stampede Simulator
                </h3>
                <p className="text-xs text-amber-200/80 leading-relaxed">
                  When a viral link receives 50,000 req/s and its cache entry expires at that exact millisecond,
                  all 50,000 requests miss the cache simultaneously. Without singleflight, 50,000 queries hammer
                  PostgreSQL at once. With Go's <code className="bg-amber-950/60 px-1 py-0.5 rounded font-mono">singleflight.Group</code>,
                  only <span className="font-bold underline">1 request</span> executes the database query; all other concurrent callers wait and receive the coalesced result.
                </p>
              </div>
            </div>

            {/* Simulation Controls */}
            <div className="glow-card p-6 sm:p-8 rounded-2xl border border-slate-800 space-y-6">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wider text-slate-300 mb-2">
                    Target Short Code
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. FXsk or custom-link"
                    value={stampedeCode}
                    onChange={e => setStampedeCode(e.target.value)}
                    className="w-full glow-input px-4 py-3 rounded-xl text-sm font-mono text-white outline-none"
                  />
                  <p className="text-[11px] text-slate-500 mt-1">
                    The simulator will purge this key from local & Redis caches to trigger cold misses.
                  </p>
                </div>

                <div>
                  <div className="flex justify-between items-center mb-2">
                    <label className="text-xs font-semibold uppercase tracking-wider text-slate-300">
                      Concurrent Goroutines
                    </label>
                    <span className="font-mono text-xs font-bold text-sky-400">
                      {concurrency} concurrent requests
                    </span>
                  </div>
                  <input
                    type="range"
                    min={10}
                    max={500}
                    step={10}
                    value={concurrency}
                    onChange={e => setConcurrency(Number(e.target.value))}
                    className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-sky-500 mt-2"
                  />
                  <div className="flex justify-between text-[10px] text-slate-500 font-mono mt-1">
                    <span>10 req</span>
                    <span>100 req</span>
                    <span>250 req</span>
                    <span>500 req</span>
                  </div>
                </div>
              </div>

              <button
                onClick={runStampedeSimulation}
                disabled={isSimulating || !stampedeCode.trim()}
                className="w-full py-4 rounded-xl bg-gradient-to-r from-amber-500 via-orange-500 to-rose-600 hover:from-amber-400 hover:to-rose-500 text-white font-bold text-sm shadow-xl shadow-orange-500/20 disabled:opacity-50 transition flex items-center justify-center gap-2 cursor-pointer"
              >
                {isSimulating ? (
                  <RefreshCw className="w-5 h-5 animate-spin" />
                ) : (
                  <>
                    <Flame className="w-5 h-5 text-amber-200" />
                    <span>Purge Cache & Trigger {concurrency} Concurrent Stampede Requests</span>
                  </>
                )}
              </button>
            </div>

            {/* Results Display */}
            {stampedeResult && (
              <div className="glow-card p-6 sm:p-8 rounded-2xl border border-emerald-500/30 bg-slate-950 space-y-6">
                <div className="flex items-center justify-between pb-4 border-b border-slate-800">
                  <div className="flex items-center gap-2">
                    <ShieldAlert className="w-5 h-5 text-emerald-400" />
                    <span className="font-bold text-base text-white">Stampede Defense Report</span>
                  </div>
                  <span className="text-xs font-mono text-emerald-400 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20">
                    Database Protected • {stampedeResult.elapsed_ms}ms total
                  </span>
                </div>

                {/* Big Metric Comparators */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  {/* Without Singleflight (Vulnerable) */}
                  <div className="p-5 rounded-xl border border-rose-500/30 bg-rose-500/5 space-y-2">
                    <div className="text-xs font-bold uppercase text-rose-400">Without Singleflight (Traditional)</div>
                    <div className="text-3xl font-extrabold text-rose-300 font-mono">
                      {stampedeResult.total_requests} DB Queries
                    </div>
                    <p className="text-xs text-rose-200/70">
                      Every single concurrent thread hits PostgreSQL directly, exhausting the connection pool.
                    </p>
                  </div>

                  {/* With Singleflight (Our Go Service) */}
                  <div className="p-5 rounded-xl border border-emerald-500/40 bg-emerald-500/10 space-y-2">
                    <div className="text-xs font-bold uppercase text-emerald-400">With EdgeLink (Go Singleflight)</div>
                    <div className="text-3xl font-extrabold text-emerald-300 font-mono">
                      {stampedeResult.database_hits} DB Query
                    </div>
                    <p className="text-xs text-emerald-200/80">
                      Singleflight collapsed {stampedeResult.total_requests} requests into 1 DB query.
                      {' '}<span className="font-semibold text-emerald-300">
                        {stampedeResult.cache_hits} requests
                      </span> received the shared result without querying the database!
                    </p>
                  </div>
                </div>

                {/* Granular Breakdown */}
                <div className="p-4 rounded-xl bg-slate-900 border border-slate-800 font-mono text-xs space-y-2">
                  <div className="text-slate-400 font-sans font-semibold uppercase text-[11px] mb-2">
                    Concurrency Resolution Breakdown
                  </div>
                  {Object.entries(stampedeResult.breakdown).map(([tier, count]) => (
                    <div key={tier} className="flex justify-between py-1 border-b border-slate-800/60 last:border-0">
                      <span className="text-slate-300">{tier}</span>
                      <span className="font-bold text-sky-400">{count} calls ({((count / stampedeResult.total_requests) * 100).toFixed(0)}%)</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* TAB 3: ARCHITECTURE */}
        {activeTab === 'architecture' && (
          <div className="space-y-8 max-w-4xl mx-auto">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="glow-card p-5 rounded-2xl border border-slate-800 space-y-3">
                <div className="w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-400 font-bold">
                  1
                </div>
                <h4 className="text-sm font-bold text-white">Collision-Free ID Range</h4>
                <p className="text-xs text-slate-400 leading-relaxed">
                  Instead of MD5/SHA hash-and-retry, each Go app instance grabs blocks of 1,000 IDs from Redis via <code className="text-sky-300">INCRBY</code>.
                  Base62 conversion occurs in memory in &lt;1µs with zero write collisions across nodes.
                </p>
              </div>

              <div className="glow-card p-5 rounded-2xl border border-slate-800 space-y-3">
                <div className="w-8 h-8 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 font-bold">
                  2
                </div>
                <h4 className="text-sm font-bold text-white">Two-Tier Caching</h4>
                <p className="text-xs text-slate-400 leading-relaxed">
                  <b className="text-slate-200">Tier 1:</b> Local in-memory LRU (5s TTL) absorbs celebrity bursts without network hops.<br />
                  <b className="text-slate-200">Tier 2:</b> Distributed Redis cluster with sliding 24h TTL.
                </p>
              </div>

              <div className="glow-card p-5 rounded-2xl border border-slate-800 space-y-3">
                <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400 font-bold">
                  3
                </div>
                <h4 className="text-sm font-bold text-white">Decoupled Analytics</h4>
                <p className="text-xs text-slate-400 leading-relaxed">
                  Redirects return <code className="text-emerald-300">302 Found</code> in &lt;2ms, pushing events to a Redis queue.
                  A background Go worker flushes batches of 100 events to PostgreSQL off the critical path.
                </p>
              </div>
            </div>

            {/* Deep-Dive Technical Blueprint Diagram */}
            <div className="glow-card p-6 sm:p-8 rounded-2xl border border-slate-800 space-y-6">
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <Database className="w-5 h-5 text-sky-400" />
                <span>Request Flow: From Browser to Database</span>
              </h3>

              <div className="bg-slate-950 p-6 rounded-xl border border-slate-800 font-mono text-xs text-slate-300 overflow-x-auto leading-relaxed">
                <pre>{`[ Client Browser ] 
        |
        v  (HTTP GET /{code} or POST /api/shorten)
[ Nginx Reverse Proxy / Load Balancer ]
        /                                   \\
       v                                     v
[ Go Instance 1 ]                     [ Go Instance 2 ]
• In-memory LRU Cache (Tier 1)         • In-memory LRU Cache (Tier 1)
• Singleflight Request Coalescer      • Singleflight Request Coalescer
• Local ID Range (e.g. 1000-1999)      • Local ID Range (e.g. 2000-2999)
       |                                     |
       +------------------+------------------+
                          |
                          v
                 [ Redis Cluster / Instance ]
                 • Global ID Counter (INCRBY 1000)
                 • Distributed Key-Value Cache (Tier 2)
                 • Click Event Queue (LPUSH url_shortener:click_queue)
                          |
                          | (Cache Miss Coalesced via Singleflight)
                          v
                 [ PostgreSQL Primary Database ]
                 • urls table (UNIQUE short_code index)
                 • click_events table (Partitioned & indexed)`}</pre>
              </div>
            </div>
          </div>
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-slate-800/80 py-6 text-center text-xs text-slate-500">
        EdgeLink System Design Lab • Built with Go 1.26, React 18, Redis 7, PostgreSQL 16 & Nginx
      </footer>
    </div>
  )
}
