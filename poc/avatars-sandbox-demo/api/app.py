import logging

import flask
import pathlib

import python_avatars as pa

logging.getLogger('werkzeug').setLevel(logging.WARN)

app = flask.Flask("avatars-api")

part_groups = {
    'facial_features': ['eyebrows', 'eyes', 'mouth', 'skin_color'],
    'hair': ['top', 'hair_color', 'facial_hair', 'facial_hair_color'],
    # ↓↓↓ remove the leading # to uncomment ↓↓↓
    # 'other': ['accessory']
    # ↑↑↑ remove the leading # to uncomment ↑↑↑
}

part_mapping = {
    'top': 'HairType',
    'hat_color': 'ClothingColor',
    'eyebrows': 'EyebrowType',
    'eyes': 'EyeType',
    'nose': 'NoseType',
    'mouth': 'MouthType',
    'facial_hair': 'FacialHairType',
    'skin_color': 'SkinColor',
    'hair_color': 'HairColor',
    'facial_hair_color': 'HairColor',
    'accessory': 'AccessoryType',
}

docker_blue = '#086DD7'
tilt_green = '#20BA31'

@app.before_first_request
def initialize():
    try:
        pa.ClothingType.DOCKER_SHIRT
    except AttributeError:
        pa.install_part(str(pathlib.Path(__file__).parent.joinpath('docker_shirt.svg')), pa.ClothingType)
    try:
        pa.ClothingType.TILT_SHIRT
    except AttributeError:
        pa.install_part(str(pathlib.Path(__file__).parent.joinpath('tilt_shirt.svg')), pa.ClothingType)


@app.route('/api/avatar')
def avatar():
    params = dict(flask.request.args)
    for p in params:
        if p not in part_mapping:
            params.pop(p, None)
            continue
        part_enum = getattr(pa, part_mapping[p])
        try:
            # enum by name, e.g. `BLACK`
            params[p] = part_enum[params[p]]
        except KeyError:
            # enum by value, e.g. `#262E33`
            params[p] = part_enum(params[p])

    clothing = 'tilt_shirt'
    clothing_color = tilt_green

    # ↓↓↓ remove the leading # to uncomment ↓↓↓
    clothing = 'docker_shirt'
    clothing_color = docker_blue
    # ↑↑↑ remove the leading # to uncomment ↑↑↑

    svg = pa.Avatar(
        style=pa.AvatarStyle.CIRCLE,
        background_color='#03C7D3',
        clothing=clothing,
        clothing_color=clothing_color,
        **params
    ).render()
    return flask.Response(svg, mimetype='image/svg+xml')


@app.route('/api/avatar/spec')
def avatar_spec():
    resp = {
        'parts': part_mapping,
        'groups': part_groups,
        'exclusions': {
            'facial_hair_color': {
                'part': 'facial_hair',
                'key': 'NONE'
            },
            'hair_color': {
                'part': 'top',
                'key': 'NONE'
            }
        },
        'values': {}
    }

    for part_type in set(resp['parts'].values()):
        values_enum = getattr(pa, part_type)
        resp['values'][part_type] = {x.name: x.value for x in values_enum}

    return flask.jsonify(resp)


@app.route('/ready')
def ready():
    return flask.Response('', status=204)
