import {useCallback, useEffect, useRef, useState} from 'react';
import {useIntl} from 'react-intl';

export const ERROR_MS = 6000;

// Runs an async click action: ignores re-entry while one is in flight (double clicks) and
// keeps the server's error text for display until ERROR_MS later.
export function useAction() {
    const intl = useIntl();
    const inFlight = useRef(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string>();
    useEffect(() => {
        if (!error) {
            return undefined;
        }
        const t = setTimeout(() => setError(undefined), ERROR_MS);
        return () => clearTimeout(t);
    }, [error]);
    const run = useCallback((fn: () => Promise<unknown>) => {
        if (inFlight.current) {
            return;
        }
        inFlight.current = true;
        setBusy(true);
        setError(undefined);
        Promise.resolve().then(fn).catch((e: unknown) => {
            setError(e instanceof Error && e.message ? e.message : intl.formatMessage({id: 'meet.error.generic', defaultMessage: 'Something went wrong. Please try again.'}));
        }).finally(() => {
            inFlight.current = false;
            setBusy(false);
        });
    }, [intl]);
    return {run, busy, error};
}

export function ActionError({error, className}: {error?: string; className?: string}) {
    if (!error) {
        return null;
    }
    return (
        <span
            role='alert'
            className={className}
        >
            {error}
        </span>
    );
}
