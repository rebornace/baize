import { useCallback, useEffect, useState } from 'react'
import {
  listChannelInboundJobs,
  retryChannelInboundJob,
  type ChannelInboundJob,
} from '../api'
import { Button } from '../components/ui'
import { INBOX, friendlyError } from '../strings'

/** Dead webhook inbound jobs (failed after retries). Admin-only settings aid. */
export function InboundDeadJobs() {
  const [jobs, setJobs] = useState<ChannelInboundJob[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [retrying, setRetrying] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setJobs(await listChannelInboundJobs())
    } catch (e) {
      setError(friendlyError(e).title)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const retry = async (id: string) => {
    setRetrying(id)
    setError(null)
    try {
      await retryChannelInboundJob(id)
      setJobs((prev) => prev.filter((job) => job.id !== id))
    } catch (e) {
      setError(friendlyError(e).title)
    } finally {
      setRetrying(null)
    }
  }

  return (
    <section className="settings-inbound-jobs" data-testid="inbound-dead-jobs">
      <div className="settings-toolbar">
        <div>
          <h3 className="settings-subheading">{INBOX.inboundDeadTitle}</h3>
          <p className="settings-muted">{INBOX.inboundDeadDesc}</p>
        </div>
        <Button type="button" variant="ghost" size="sm" disabled={loading} onClick={() => void load()}>
          {INBOX.inboundDeadRefresh}
        </Button>
      </div>
      {loading && <p className="settings-muted">…</p>}
      {!loading && error && (
        <p className="ui-inline-error" role="alert">
          {error}
        </p>
      )}
      {!loading && !error && jobs.length === 0 && (
        <p className="settings-muted">{INBOX.inboundDeadEmpty}</p>
      )}
      {!loading && jobs.length > 0 && (
        <ul className="inbound-job-list">
          {jobs.map((job) => (
            <li className="inbound-job-row" key={job.id}>
              <div className="inbound-job-body">
                <p className="inbound-job-meta">
                  {INBOX.inboundDeadMeta(job.channel, job.attempt, job.max_attempts)}
                </p>
                {job.last_error && <p className="inbound-job-error">{job.last_error}</p>}
              </div>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={retrying === job.id}
                onClick={() => void retry(job.id)}
              >
                {retrying === job.id ? INBOX.inboundDeadRetrying : INBOX.inboundDeadRetry}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
