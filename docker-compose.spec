# -*- mode: python -*-

block_cipher = None

a = Analysis(['bin/docker-compose'],
             pathex=['.'],
             hiddenimports=[],
             hookspath=None,
             runtime_hooks=None,
             cipher=block_cipher)

pyz = PYZ(a.pure, cipher=block_cipher)

exe = EXE(pyz,
          a.scripts,
          a.binaries,
          a.zipfiles,
          a.datas,
          [
            (
                'compose/config/config_schema_v1.json',
                'compose/config/config_schema_v1.json',
                'DATA'
            ),
            (
                'compose/config/compose_spec.json',
                'compose/config/compose_spec.json',
                'DATA'
            ),
            (
                'compose/GITSHA',
                'compose/GITSHA',
                'DATA'
            )
          ],

          name='docker-compose',
          debug=False,
          strip=None,
          upx=True,
          console=True,
          bootloader_ignore_signals=True)
