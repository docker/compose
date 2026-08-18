import React from 'react'
import 'fg-select-css/src/select-css.css';

const specialCaseLabels = {
    'EYE_ROLL': 'eye roll',
    'LONG_NOT_TOO_LONG': 'medium',
    'CAESAR_SIDE_PART': 'caesar (side part)'
}

function sanitizeName(name) {
    return name.replaceAll('_', ' ')
}

// sanitizeLabel changes strings like "SAD_CONCERNED_NATURAL" -> "sad (concerned, natural)"
function sanitizeLabel(label) {
    // this string manipulation is not perfect, so we've hardcoded some
    if (label in specialCaseLabels) {
        return specialCaseLabels[label];
    }
    label = label.toLowerCase();
    const splitIndex = label.indexOf('_');
    if (splitIndex === -1) {
        return label;
    }
    const name = label.substr(0, splitIndex)
    const desc = label.substr(splitIndex+1, label.length - splitIndex).replaceAll('_', ', ')
    return `${name} (${desc})`
}

function Parts(props) {
    if (!props.spec) {
        return null;
    }
    return (
        <section className={"avatar-editor"}>
            <h2 className={"sr-only"}>Editor</h2>
            { Object.keys(props.spec.groups).map(groupName =>
                <PartGroup
                    key={`group-${groupName}`}
                    name={groupName}
                    parts={props.spec.groups[groupName]}
                    {...props}
                />)}
        </section>
    )
}

function PartGroup(props) {
    return <div className={"part-group"}>
        <h3>{sanitizeName(props.name)}</h3>
        { props.parts.map(partName => {
            const exclusion = props.spec.exclusions[partName];
            if (exclusion && props.choices[exclusion.part] === props.spec.values[props.spec.parts[exclusion.part]][exclusion.key]) {
                return null;
            }

            return <PartPicker
                key={partName}
                onChange={props.onChange}
                name={partName}
                type={props.spec.parts[partName]}
                current={props.choices[partName]}
                values={props.spec.values[props.spec.parts[partName]]}
            />})}
    </div>;
}

function PartPicker(props) {
    if (props.type.endsWith("Color")) {
        return <ColorPicker {...props} />
    }

    const onChange = (e) => {
        props.onChange(props.name, e.target.value)
    }

    return <div className={"item-picker picker"}>
        <label htmlFor={`part-${props.name}`} className={"part-title"}>{sanitizeName(props.name)}</label>
        <select id={`part-${props.name}`} onChange={onChange} value={props.current} className={"select-css"}>
            {Object.keys(props.values).map(itemName =>
            <option key={`${props.name}=${itemName}`} value={props.values[itemName]}>{sanitizeLabel(itemName)}</option>)}
        </select>
    </div>
}

function ColorPicker(props) {
    const onChange = (e) => {
        props.onChange(props.name, e.target.value)
    }

    return <div className="color-picker picker">
        <fieldset id={`part-${props.name}`} value={props.current} onChange={onChange}>
            <legend className={"part-title"}>{sanitizeName(props.name)}</legend>
            {Object.keys(props.values).map(colorName =>
                <label key={`${props.name}-${colorName}`} style={{backgroundColor: props.values[colorName]}} aria-label={sanitizeName(colorName)}>
                    <input type="radio" value={props.values[colorName]} />
                </label>)}
        </fieldset>
    </div>
}

export default Parts
