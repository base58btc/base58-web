/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./templates/*.tmpl", "./templates/*/*.tmpl"],
  theme: {
    extend: { 
	colors : {
		bitcoin: {
			DEFAULT: '#FFA800',		   
		},
		btcgold: {
			DEFAULT: '#FF7E01',
	        }
	},
	fontFamily: {
	    bitcoin: ['Ubuntu-BoldItalic', 'sans-serif'],
            arial: ['Arial'],
        },
    },
  },
  plugins: [
	  require('@tailwindcss/forms'),
	  require('@tailwindcss/typography'),
    require('@tailwindcss/aspect-ratio'),
  ],
}
