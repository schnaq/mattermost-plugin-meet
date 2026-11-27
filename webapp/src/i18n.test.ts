import de from '../i18n/de.json';
import en from '../i18n/en.json';

test('en.json and de.json have the same message ids', () => {
    expect(Object.keys(de).sort()).toEqual(Object.keys(en).sort());
});
