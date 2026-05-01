import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Route, Routes } from 'react-router-dom'

import './tokens.css'

import Root from './routes/root'
import Handshake from './routes/handshake'
import Workspaces from './routes/workspaces'
import WorkspaceDetail from './routes/workspace-detail'
import CheckpointDetail from './routes/checkpoint-detail'
import Activity from './routes/activity'
import Sessions from './routes/sessions'
import Tools from './routes/tools'
import Receipt from './routes/receipt'
import Why from './routes/why'
import NotFound from './routes/notfound'

const root = document.getElementById('root')!

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Root />} />
        <Route path="/handshake" element={<Handshake />} />
        <Route path="/workspaces" element={<Workspaces />} />
        <Route path="/workspaces/:id" element={<WorkspaceDetail />} />
        <Route path="/checkpoints/:id" element={<CheckpointDetail />} />
        <Route path="/activity" element={<Activity />} />
        <Route path="/sessions" element={<Sessions />} />
        <Route path="/tools" element={<Tools />} />
        <Route path="/receipts/:hash" element={<Receipt />} />
        <Route path="/why/:actionId" element={<Why />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
