import React, {useEffect, useState} from 'react';

const pad = (n: number) => String(n).padStart(2, '0');

// Local mm:ss since startedAt; the 1 s interval runs only while this is mounted.
export function Elapsed({startedAt, className}: {startedAt: number; className?: string}) {
    const [now, setNow] = useState(Date.now);
    useEffect(() => {
        const t = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(t);
    }, []);
    const s = Math.max(0, Math.floor((now - startedAt) / 1000));
    return <span className={className}>{`${pad(Math.floor(s / 60))}:${pad(s % 60)}`}</span>;
}
