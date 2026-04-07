import { useState } from 'react'
import Landing from './components/Landing'
import IDE from './components/IDE'

export default function App() {
  const [session, setSession] = useState(null)
  // session: { code, userID, role }

  if (!session) {
    return <Landing onJoin={setSession} />
  }

  return <IDE session={session} />
}
