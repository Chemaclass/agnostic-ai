import { useId, useState } from 'react'
import type { NativeServer } from '../contract'
import { MCPSpecImportButton, MCPSpecPreviewPanel, useMCPSpecImport, type MCPSpecSaved } from './MCPSpecAction'
import { SectionHeading } from './SectionHeading'
import { nativeSearchText } from '../model/section-tabs'
import { joinAnd } from '../model/counts'
import { CopyButton, DetailsToggle, RowActions, Select, ToolAvatar } from './ui'

export function NativeServers({ servers, matches, filtering, onSaved }: { servers: NativeServer[]; matches: (parts: (string | undefined)[]) => boolean; filtering: boolean; onSaved: (saved: MCPSpecSaved) => void }) {
  const visible = servers.filter((server) => matches(nativeSearchText(server)))
  return (
    <section className="block" aria-labelledby="tools-native">
      <SectionHeading id="tools-native" hidden>
        Found in tools <span className="muted kind-count">{filtering ? `${visible.length} of ${servers.length} match` : servers.length}</span>
      </SectionHeading>
      <p className="muted native-intro">These servers live only in a tool’s own config. Import one to share it with every tool you use.</p>
      {visible.length === 0 && filtering ? <p className="muted">No matches.</p> : (
        <ul className="item-list native-list">
          {visible.map((server) => <NativeRow key={server.name} server={server} onSaved={onSaved} />)}
        </ul>
      )}
    </section>
  )
}

function NativeRow({ server, onSaved }: { server: NativeServer; onSaved: (saved: MCPSpecSaved) => void }) {
  const [selected, setSelected] = useState(0)
  const [open, setOpen] = useState(false)
  const detailsId = useId()
  const flow = useMCPSpecImport(server, server.holders[selected] ?? server.holders[0], onSaved)
  const targets = [...new Map(server.holders.map((holder) => [holder.target, holder.label])).entries()]
  return (
    <li className="item-row row-hover native-row">
      <div className="native-head">
        <strong className="native-name">{server.name}</strong>
        <span className="native-tools">
          <span className="tool-list" aria-hidden="true">{targets.map(([target, label]) => <ToolAvatar key={target} target={target} label={label} />)}</span>
          <span>In {joinAnd(targets.map(([, label]) => label))}</span>
        </span>
        <span className="native-actions">
          <RowActions label={server.name}>
            <DetailsToggle open={open} controls={detailsId} label={server.name} onToggle={() => setOpen(!open)} />
          </RowActions>
          {flow && <MCPSpecImportButton server={server} flow={flow} />}
        </span>
      </div>
      <dl id={detailsId} className="row-details native-details" hidden={!open}>
        {flow && server.holders.length > 1 && (
          <div>
            <dt>Import from</dt>
            <dd>
              <Select aria-label={`Source for ${server.name}`} value={selected} disabled={!!flow.pending} onChange={(event) => setSelected(Number(event.target.value))}>
                {server.holders.map((holder, index) => <option key={`${holder.target}:${holder.path}:${holder.scope}`} value={index}>{holder.label}: {holder.path} ({holder.scope === 'user' ? 'your user settings' : 'this project'})</option>)}
              </Select>
            </dd>
          </div>
        )}
        <div>
          <dt>Found in</dt>
          <dd className="native-paths">
            {server.holders.map((holder) => (
              <span key={`${holder.target}:${holder.path}:${holder.scope}`} title={`${holder.label}, ${holder.scope === 'user' ? 'every project' : 'this project'}`}>
                <code className="item-path">{holder.path}</code>{holder.transport && <span className="native-transport">{holder.transport}</span>}
              </span>
            ))}
          </dd>
        </div>
        {server.import && (
          <div>
            <dt>In a terminal</dt>
            <dd className="native-command">
              <code className="code">{server.import}</code>
              <CopyButton text={server.import} label="Copy command" ariaLabel={`Copy import command for ${server.name}`} />
            </dd>
          </div>
        )}
      </dl>
      {flow && <MCPSpecPreviewPanel server={server} flow={flow} />}
    </li>
  )
}
