import { ArrowRight, BriefcaseBusiness, GraduationCap, MapPin, Search, ShieldCheck, UsersRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Link } from 'react-router-dom'
import { useAuth } from '@/app/authContext'

const areas = [
  { icon: BriefcaseBusiness, label: 'Career paths', detail: 'See where mathematics education can take you.' },
  { icon: UsersRound, label: 'Real connections', detail: 'Build a network rooted in a shared campus.' },
  { icon: ShieldCheck, label: 'Trusted community', detail: 'Alumni membership is reviewed before listing.' },
]

function App() {
  const { status, login, register, logout } = useAuth()
  return (
    <div className="site-shell">
      <header className="site-header">
        <div className="container header-inner">
          <a className="brand" href="#top" aria-label="AlumniBridge home">
            <span className="brand-mark" aria-hidden="true"><span /><span /><span /></span>
            <span>Alumni<span className="brand-accent">Bridge</span></span>
          </a>
          <nav className="desktop-nav" aria-label="Main navigation">
            <a href="#about">About</a>
            <Link to="/alumni">Alumni Directory</Link>
            <a href="#how-it-works">How it works</a>
          </nav>
          <div className="header-action">{status === 'authenticated' ? <><Link className="text-link" to="/dashboard">My home</Link><Button variant="outline" onClick={() => void logout()}>Sign out</Button></> : <Button variant="outline" onClick={() => void login()} disabled={status === 'loading' || status === 'error'}>Sign in</Button>}</div>
          <nav className="mobile-menu" aria-label="Mobile navigation"><Link to="/alumni">Alumni Directory</Link>{status === 'authenticated' ? <Link to="/dashboard">My home</Link> : <button type="button" onClick={() => void login()} disabled={status === 'loading' || status === 'error'}>Sign in</button>}</nav>
        </div>
      </header>

      <main id="top">
        <section className="hero container" aria-labelledby="hero-title">
          <div className="hero-copy">
            <div className="eyebrow"><span className="eyebrow-dot" /> BUILT FOR THE NEXT CONNECTION</div>
            <h1 id="hero-title">Your next chapter starts with <em>who you know.</em></h1>
            <p className="hero-description">A place for students and alumni to share experience, explore career paths, and stay connected to the community that shaped them.</p>
            <div className="hero-actions">
              {status === 'authenticated' ? <Link className="join-link" to="/profile">Your professional profile <ArrowRight size={17} /></Link> : <Button size="lg" onClick={() => void register()} disabled={status === 'loading' || status === 'error'}>Join the community <ArrowRight size={17} /></Button>}
              <a className="text-link" href="#about">Get to know us <ArrowRight size={17} /></a>
            </div>
            {status === 'loading' && <p className="auth-note" role="status">Checking your sign-in status…</p>}
            {status === 'error' && <p className="auth-note" role="alert">Sign-in is temporarily unavailable. Please try again later.</p>}
            <div className="launch-note"><span className="launch-avatars" aria-hidden="true"><i>AB</i><i>UH</i><i>ME</i></span><span>Beginning with Mathematics Education at <strong>UHAMKA</strong></span></div>
          </div>

          <div className="hero-art" aria-label="Illustration of a connected alumni community" role="img">
            <div className="art-grid" />
            <div className="orbit orbit-one" /><div className="orbit orbit-two" />
            <div className="connection-line line-one" /><div className="connection-line line-two" /><div className="connection-line line-three" />
            <div className="art-card card-main"><span className="avatar avatar-main">AB</span><div><strong>Find your people.</strong><small>Grow together, wherever life takes you.</small></div></div>
            <div className="art-card card-top"><span className="avatar avatar-gold"><GraduationCap size={22} /></span><div><strong>Shared roots</strong><small>UHAMKA community</small></div></div>
            <div className="art-card card-bottom"><span className="avatar avatar-blue"><BriefcaseBusiness size={20} /></span><div><strong>Many paths</strong><small>One connected network</small></div></div>
            <span className="art-spark spark-one" /><span className="art-spark spark-two" />
          </div>
        </section>

        <section className="community-band" id="community" aria-label="Launching community">
          <div className="container band-inner"><div className="band-icon"><MapPin size={22} /></div><p><strong>Rooted at UHAMKA.</strong> Designed to connect students and alumni across careers, cities, and generations.</p><span className="band-label">Our first community <ArrowRight size={16} /></span></div>
        </section>

        <section className="about-section container" id="about">
          <div className="section-heading"><span className="section-kicker">WHY ALUMNIBRIDGE</span><h2>More than a directory.<br /><em>A way forward, together.</em></h2><p>Meaningful professional relationships often start with something simple: a shared beginning.</p></div>
          <div className="feature-grid">{areas.map(({ icon: Icon, label, detail }, index) => <article className="feature-card" key={label}><span className="feature-number">0{index + 1}</span><div className="feature-icon"><Icon size={24} strokeWidth={1.8} /></div><h3>{label}</h3><p>{detail}</p></article>)}</div>
        </section>

        <section className="coming-section" id="how-it-works"><div className="container coming-inner"><div><span className="section-kicker">ALUMNI DIRECTORY</span><h2>A network built around <em>possibility.</em></h2><p>Explore profiles that verified alumni choose to share. Search professional experience, skills and academic details they explicitly publish.</p><Button asChild><Link to="/alumni">Explore alumni <ArrowRight size={17} /></Link></Button></div><div className="preview-card"><div className="preview-top"><span><Search size={18} /> Alumni discovery</span><span className="preview-pill">OPEN NOW</span></div><div className="preview-line long" /><div className="preview-line medium" /><div className="preview-line short" /><div className="preview-footer"><span className="preview-circle" /><span className="preview-circle" /><span className="preview-circle" /><span>Find a path worth exploring</span></div></div></div></section>
      </main>

      <footer className="site-footer"><div className="container footer-inner"><a className="brand" href="#top"><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span><span>Alumni<span className="brand-accent">Bridge</span></span></a><p>From a shared beginning, toward new opportunities.</p><span>Made for a connected future.</span></div></footer>
    </div>
  )
}

export default App
