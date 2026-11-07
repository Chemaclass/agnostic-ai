import type { NativeServer } from '../contract'
import { EditAudienceSwitch } from './EditAudience'
import { MCPSpecAction, type MCPSpecSaved } from './MCPSpecAction'
import { SectionHeading } from './SectionHeading'
import { nativeSearchText } from '../model/section-tabs'
import { Chip, Code } from './ui'

export function NativeServers({ servers, matches, filtering, onSaved }: { servers: NativeServer[]; matches: (parts: (string | undefined)[]) => boolean; filtering: boolean; onSaved: (saved: MCPSpecSaved) => void }) {
  const visible = servers.filter((server) => matches(nativeSearchText(server)))
  return (
    <section className="block" aria-labelledby="tools-native">
      <SectionHeading id="tools-native">
        Found in tools <span className="muted kind-count">{filtering ? `${visible.length} of ${servers.length} match` : servers.length}</span>
      </SectionHeading>
      <p className="muted">These servers work only in the tool that holds them. agnostic-ai has no file for them, so it does not share them with your other tools.</p>
      {visible.some((server) => server.holders.length > 0) && <EditAudienceSwitch />}
      {visible.length === 0 && filtering ? <p className="muted">No matches.</p> : (
        <ul className="item-list native-list">
          {visible.map((s) => {
            const tools = [...new Set(s.holders.map((h) => h.label))]
            const transports = [...new Set(s.holders.flatMap((h) => (h.transport ? [h.transport] : [])))]
            return (
              <li key={s.name} className="item-row native-row">
                <div className="item-heading">
                  <div className="item-main">
                    <strong>{s.name}</strong>
                    <span className="muted item-description">{tools.length === 1 ? `${tools[0]} only` : tools.join(', ')}</span>
                    {s.holders.map((h) => (
                      <code key={`${h.target}:${h.path}:${h.scope}`} className="item-path" title={`${h.label}, ${h.scope === 'user' ? 'every project' : 'this project'}`}>{h.path}</code>
                    ))}
                  </div>
                  <div className="item-meta">
                    {transports.map((t) => <Chip key={t}>{t}</Chip>)}
                  </div>
                </div>
                {s.import && (
                  <p className="muted tool-state">
                    Run <Code>{s.import}</Code> to add it to your shared setup.
                  </p>
                )}
                <MCPSpecAction server={s} onSaved={onSaved} />
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
