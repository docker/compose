import React, {useEffect, useState} from 'react'
import './App.css'
import Parts from "./Parts";

function App() {
    const [error, setError] = useState(null);
    const [loading, setLoading] = useState(true);
    const [spec, setSpec] = useState(null);
    const [partChoices, setPartChoices] = useState({});
    const [avatarURL, setAvatarURL] = useState(null);

    const randomizeChoices = (spec) => {
        if (!spec) {
            return;
        }

        const parts = {};
        Object.keys(spec.parts).forEach(partName => {
            // only pick a value for parts that exist in a group within the editor
            // to prevent setting a part you can't change
            if (Object.values(spec.groups).some(g => g.includes(partName))) {
                const partType = spec.parts[partName];
                const values = Object.values(spec.values[partType]);
                parts[partName] = values[Math.floor(Math.random() * values.length)];
            }
        })
        setPartChoices(parts);
    };

    useEffect(() => {
        setLoading(true)
        fetch("/api/avatar/spec")
            .then(res => res.json())
            .then(result => {
                setSpec(result);
                // lock in initial choices
                randomizeChoices(result)
                setLoading(false)
            }, error => {
                setError(error)
                setLoading(false)
            })
    }, []);

    const onPartChoice = (name, value) => {
        setPartChoices({...partChoices, [name]: value})
    }

    useEffect(() => {
        if (loading) {
            setAvatarURL(null);
            return;
        }
        setAvatarURL('/api/avatar?' + new URLSearchParams(partChoices));
    }, [partChoices, loading]);

    if (error) {
        return <div>Failed to load: {error}</div>
    } else if (loading) {
        return <div>Loading...</div>
    } else {
        return <>
            <div className={"avatar-wrapper"}>
                <img className={"avatar"} src={avatarURL} alt="Your Tilt Avatar"/>
            </div>
            <Parts spec={spec} choices={partChoices} onChange={onPartChoice}/>
        </>
    }
}

export default App
